package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Seraf-seraf/tasks/internal/domain"
	"github.com/Seraf-seraf/tasks/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
)

type Server struct {
	s    *service.Service
	auth *jwtauth.JWTAuth
	req  *prometheus.CounterVec
	errs *prometheus.CounterVec
	dur  *prometheus.HistogramVec
	mu   sync.Mutex
	lim  map[int64]*rate.Limiter
}

func New(s *service.Service, secret string) *Server {
	return &Server{
		s:    s,
		auth: jwtauth.New("HS256", []byte(secret), nil),
		lim:  map[int64]*rate.Limiter{},
		req: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "requests",
		}, []string{"path", "code"}),
		errs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_errors_total",
			Help: "errors",
		}, []string{"path", "code"}),
		dur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "duration",
		}, []string{"path"}),
	}
}

func (h *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, h.metrics)
	r.Handle("/metrics", promhttp.Handler())

	registerCollector(h.req)
	registerCollector(h.errs)
	registerCollector(h.dur)

	strict := NewStrictHandlerWithOptions(
		h,
		[]StrictMiddlewareFunc{h.authRateLimit},
		StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeError(w, http.StatusBadRequest, err.Error())
			},
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeError(w, http.StatusInternalServerError, err.Error())
			},
		},
	)
	HandlerFromMux(strict, r)

	return r
}

func (h *Server) metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(rw, r)
		code := strconv.Itoa(rw.Status())
		h.req.WithLabelValues(r.URL.Path, code).Inc()
		if rw.Status() >= http.StatusBadRequest {
			h.errs.WithLabelValues(r.URL.Path, code).Inc()
		}
		h.dur.WithLabelValues(r.URL.Path).Observe(time.Since(start).Seconds())
	})
}

func registerCollector(c prometheus.Collector) {
	if err := prometheus.Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if !errors.As(err, &already) {
			panic(err)
		}
	}
}

func (h *Server) authRateLimit(next StrictHandlerFunc, operationID string) StrictHandlerFunc {
	if operationID == "Login" || operationID == "Register" {
		return next
	}
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		token, err := jwtauth.VerifyRequest(h.auth, r, jwtauth.TokenFromHeader, jwtauth.TokenFromCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return nil, nil
		}
		ctx = jwtauth.NewContext(ctx, token, nil)
		r = r.WithContext(ctx)
		if !h.allow(h.uid(r)) {
			writeError(w, http.StatusTooManyRequests, "rate limit")
			return nil, nil
		}
		return next(ctx, w, r, request)
	}
}

func (h *Server) uid(r *http.Request) int64 {
	_, c, _ := jwtauth.FromContext(r.Context())
	v, _ := strconv.ParseInt(fmt.Sprint(c["uid"]), 10, 64)
	return v
}

func (h *Server) allow(uid int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	l := h.lim[uid]
	if l == nil {
		l = rate.NewLimiter(rate.Every(time.Minute/100), 100)
		h.lim[uid] = l
	}
	return l.Allow()
}

func (h *Server) Register(ctx context.Context, request RegisterRequestObject) (RegisterResponseObject, error) {
	if request.Body == nil || strings.TrimSpace(string(request.Body.Email)) == "" || strings.TrimSpace(request.Body.Name) == "" || request.Body.Password == "" {
		return Register422JSONResponse{UnprocessableEntityJSONResponse: validationError()}, nil
	}
	id, err := h.s.Register(ctx, string(request.Body.Email), request.Body.Name, request.Body.Password)
	if err != nil {
		if isConflict(err) {
			return Register409JSONResponse{ConflictJSONResponse: conflictError()}, nil
		}
		return Register400JSONResponse{BadRequestJSONResponse: badRequestFromError(err)}, nil
	}
	return Register201JSONResponse{Id: id}, nil
}

func (h *Server) Login(ctx context.Context, request LoginRequestObject) (LoginResponseObject, error) {
	if request.Body == nil || strings.TrimSpace(string(request.Body.Email)) == "" || request.Body.Password == "" {
		return Login422JSONResponse{UnprocessableEntityJSONResponse: validationError()}, nil
	}
	u, err := h.s.Login(ctx, string(request.Body.Email), request.Body.Password)
	if err != nil {
		return Login401JSONResponse{UnauthorizedJSONResponse: unauthorizedError()}, nil
	}
	_, token, err := h.auth.Encode(map[string]any{"uid": u.ID, "exp": time.Now().Add(24 * time.Hour).Unix()})
	if err != nil {
		return Login500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return Login200JSONResponse{Token: token}, nil
}

func (h *Server) CreateTeam(ctx context.Context, request CreateTeamRequestObject) (CreateTeamResponseObject, error) {
	if request.Body == nil || strings.TrimSpace(request.Body.Name) == "" {
		return CreateTeam422JSONResponse{UnprocessableEntityJSONResponse: validationError()}, nil
	}
	id, err := h.s.CreateTeam(ctx, request.Body.Name, uidFromContext(ctx))
	if err != nil {
		return CreateTeam400JSONResponse{BadRequestJSONResponse: badRequestFromError(err)}, nil
	}
	return CreateTeam201JSONResponse{Id: id}, nil
}

func (h *Server) ListTeams(ctx context.Context, _ ListTeamsRequestObject) (ListTeamsResponseObject, error) {
	x, err := h.s.Teams(ctx, uidFromContext(ctx))
	if err != nil {
		return ListTeams500JSONResponse{InternalErrorJSONResponse: internalFromError(err)}, nil
	}
	return ListTeams200JSONResponse(mapTeams(x)), nil
}

func (h *Server) InviteUser(ctx context.Context, request InviteUserRequestObject) (InviteUserResponseObject, error) {
	if request.Body == nil || request.Body.UserId <= 0 || request.Body.Role == "" {
		return InviteUser422JSONResponse{UnprocessableEntityJSONResponse: validationError()}, nil
	}
	email := ""
	if request.Body.Email != nil {
		email = string(*request.Body.Email)
	}
	err := h.s.Invite(ctx, uidFromContext(ctx), request.Id, request.Body.UserId, domain.Role(request.Body.Role), email)
	if err != nil {
		if isForbidden(err) {
			return InviteUser403JSONResponse{ForbiddenJSONResponse: forbiddenError()}, nil
		}
		if isConflict(err) {
			return InviteUser409JSONResponse{ConflictJSONResponse: conflictError()}, nil
		}
		return InviteUser400JSONResponse{BadRequestJSONResponse: badRequestFromError(err)}, nil
	}
	return InviteUser200JSONResponse{Status: "ok"}, nil
}

func (h *Server) CreateTask(ctx context.Context, request CreateTaskRequestObject) (CreateTaskResponseObject, error) {
	if request.Body == nil || request.Body.TeamId <= 0 || strings.TrimSpace(request.Body.Title) == "" {
		return CreateTask422JSONResponse{UnprocessableEntityJSONResponse: validationError()}, nil
	}
	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}
	status := domain.StatusTodo
	if request.Body.Status != nil {
		status = domain.TaskStatus(*request.Body.Status)
	}
	id, err := h.s.CreateTask(ctx, domain.Task{
		TeamID:      request.Body.TeamId,
		Title:       request.Body.Title,
		Description: description,
		Status:      status,
		AssigneeID:  request.Body.AssigneeId,
		CreatedBy:   uidFromContext(ctx),
	})
	if err != nil {
		if isForbidden(err) || strings.Contains(err.Error(), "member required") {
			return CreateTask403JSONResponse{ForbiddenJSONResponse: forbiddenError()}, nil
		}
		return CreateTask400JSONResponse{BadRequestJSONResponse: badRequestFromError(err)}, nil
	}
	return CreateTask201JSONResponse{Id: id}, nil
}

func (h *Server) ListTasks(ctx context.Context, request ListTasksRequestObject) (ListTasksResponseObject, error) {
	limit := 20
	if request.Params.Limit != nil && *request.Params.Limit > 0 && *request.Params.Limit <= 100 {
		limit = *request.Params.Limit
	}
	offset := 0
	if request.Params.Offset != nil && *request.Params.Offset > 0 {
		offset = *request.Params.Offset
	}
	status := ""
	if request.Params.Status != nil {
		status = string(*request.Params.Status)
	}
	x, err := h.s.ListTasks(ctx, uidFromContext(ctx), request.Params.TeamId, status, request.Params.AssigneeId, limit, offset)
	if err != nil {
		if isForbidden(err) {
			return ListTasks403JSONResponse{ForbiddenJSONResponse: forbiddenError()}, nil
		}
		return ListTasks400JSONResponse{BadRequestJSONResponse: badRequestFromError(err)}, nil
	}
	return ListTasks200JSONResponse{Body: mapTasks(x)}, nil
}

func (h *Server) UpdateTask(ctx context.Context, request UpdateTaskRequestObject) (UpdateTaskResponseObject, error) {
	if request.Body == nil {
		return UpdateTask400JSONResponse{BadRequestJSONResponse: badRequestError()}, nil
	}
	patch := domain.Task{AssigneeID: request.Body.AssigneeId}
	if request.Body.Title != nil {
		patch.Title = *request.Body.Title
	}
	if request.Body.Description != nil {
		patch.Description = *request.Body.Description
	}
	if request.Body.Status != nil {
		patch.Status = domain.TaskStatus(*request.Body.Status)
	}
	err := h.s.UpdateTask(ctx, uidFromContext(ctx), request.Id, patch)
	if err != nil {
		if isForbidden(err) {
			return UpdateTask403JSONResponse{ForbiddenJSONResponse: forbiddenError()}, nil
		}
		if isNotFound(err) {
			return UpdateTask404JSONResponse{NotFoundJSONResponse: notFoundError()}, nil
		}
		return UpdateTask400JSONResponse{BadRequestJSONResponse: badRequestFromError(err)}, nil
	}
	return UpdateTask200JSONResponse{Status: "ok"}, nil
}

func (h *Server) TaskHistory(ctx context.Context, request TaskHistoryRequestObject) (TaskHistoryResponseObject, error) {
	x, err := h.s.History(ctx, uidFromContext(ctx), request.Id)
	if err != nil {
		if isForbidden(err) {
			return TaskHistory403JSONResponse{ForbiddenJSONResponse: forbiddenError()}, nil
		}
		if isNotFound(err) {
			return TaskHistory404JSONResponse{NotFoundJSONResponse: notFoundError()}, nil
		}
		return TaskHistory500JSONResponse{InternalErrorJSONResponse: internalFromError(err)}, nil
	}
	return TaskHistory200JSONResponse(mapHistory(x)), nil
}

func (h *Server) Reports(ctx context.Context, _ ReportsRequestObject) (ReportsResponseObject, error) {
	a, b, c, err := h.s.Reports(ctx)
	if err != nil {
		return Reports500JSONResponse{InternalErrorJSONResponse: internalFromError(err)}, nil
	}
	return Reports200JSONResponse{
		TeamStats:        mapTeamStats(a),
		TopCreators:      mapTopCreators(b),
		InvalidAssignees: mapTasks(c),
	}, nil
}

func uidFromContext(ctx context.Context) int64 {
	_, c, _ := jwtauth.FromContext(ctx)
	v, _ := strconv.ParseInt(fmt.Sprint(c["uid"]), 10, 64)
	return v
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

func errorResponse(err error) ErrorResponse {
	return ErrorResponse{Error: err.Error()}
}

func badRequestFromError(err error) BadRequestJSONResponse {
	return BadRequestJSONResponse(errorResponse(err))
}

func internalFromError(err error) InternalErrorJSONResponse {
	return InternalErrorJSONResponse(errorResponse(err))
}

func badRequestError() BadRequestJSONResponse {
	return BadRequestJSONResponse{Error: "bad request"}
}

func validationError() UnprocessableEntityJSONResponse {
	return UnprocessableEntityJSONResponse{Error: "validation error"}
}

func unauthorizedError() UnauthorizedJSONResponse {
	return UnauthorizedJSONResponse{Error: "unauthorized"}
}

func forbiddenError() ForbiddenJSONResponse {
	return ForbiddenJSONResponse{Error: "forbidden"}
}

func notFoundError() NotFoundJSONResponse {
	return NotFoundJSONResponse{Error: "not found"}
}

func conflictError() ConflictJSONResponse {
	return ConflictJSONResponse{Error: "conflict"}
}

func internalError() InternalErrorJSONResponse {
	return InternalErrorJSONResponse{Error: "internal error"}
}

func isForbidden(err error) bool {
	return strings.Contains(err.Error(), "forbidden")
}

func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "not found")
}

func isConflict(err error) bool {
	return strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "1062")
}

func mapTeams(in []domain.Team) []Team {
	out := make([]Team, 0, len(in))
	for _, x := range in {
		role := Role(x.Role)
		out = append(out, Team{
			Id:        x.ID,
			Name:      x.Name,
			CreatedBy: x.CreatedBy,
			Role:      &role,
			CreatedAt: x.CreatedAt,
		})
	}
	return out
}

func mapTasks(in []domain.Task) []Task {
	out := make([]Task, 0, len(in))
	for _, x := range in {
		out = append(out, mapTask(x))
	}
	return out
}

func mapTask(x domain.Task) Task {
	return Task{
		Id:          x.ID,
		TeamId:      x.TeamID,
		Title:       x.Title,
		Description: x.Description,
		Status:      TaskStatus(x.Status),
		AssigneeId:  x.AssigneeID,
		CreatedBy:   x.CreatedBy,
		CreatedAt:   x.CreatedAt,
		UpdatedAt:   x.UpdatedAt,
	}
}

func mapHistory(in []domain.TaskHistory) []TaskHistory {
	out := make([]TaskHistory, 0, len(in))
	for _, x := range in {
		out = append(out, TaskHistory{
			Id:        x.ID,
			TaskId:    x.TaskID,
			ChangedBy: x.ChangedBy,
			Field:     x.Field,
			OldValue:  x.OldValue,
			NewValue:  x.NewValue,
			CreatedAt: x.CreatedAt,
		})
	}
	return out
}

func mapTeamStats(in []domain.TeamStats) []TeamStats {
	out := make([]TeamStats, 0, len(in))
	for _, x := range in {
		out = append(out, TeamStats{
			TeamId:        x.TeamID,
			Name:          x.Name,
			Members:       x.Members,
			DoneLast7Days: x.DoneLast7Days,
		})
	}
	return out
}

func mapTopCreators(in []domain.TopCreator) []TopCreator {
	out := make([]TopCreator, 0, len(in))
	for _, x := range in {
		out = append(out, TopCreator{
			TeamId:       x.TeamID,
			UserId:       x.UserID,
			Email:        openapi_types.Email(x.Email),
			CreatedTasks: x.CreatedTasks,
			Rank:         x.Rank,
		})
	}
	return out
}
