package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"net/http"
	"strconv"
	"strings"
	"taskservice/internal/domain"
	"taskservice/internal/service"
	"time"
)

type Handler struct {
	s    *service.Service
	auth *jwtauth.JWTAuth
	req  *prometheus.CounterVec
	dur  *prometheus.HistogramVec
	lim  map[int64]*rate.Limiter
}

func New(s *service.Service, secret string) *Handler {
	h := &Handler{s: s, auth: jwtauth.New("HS256", []byte(secret), nil), lim: map[int64]*rate.Limiter{}, req: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total", Help: "requests"}, []string{"path", "code"}), dur: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "duration"}, []string{"path"})}
	prometheus.MustRegister(h.req, h.dur)
	return h
}
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, h.metrics)
	r.Handle("/metrics", promhttp.Handler())
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/register", h.register)
		r.Post("/login", h.login)
		r.Group(func(r chi.Router) {
			r.Use(jwtauth.Verifier(h.auth), jwtauth.Authenticator(h.auth), h.rateLimit)
			r.Post("/teams", h.createTeam)
			r.Get("/teams", h.teams)
			r.Post("/teams/{id}/invite", h.invite)
			r.Post("/tasks", h.createTask)
			r.Get("/tasks", h.tasks)
			r.Put("/tasks/{id}", h.updateTask)
			r.Get("/tasks/{id}/history", h.history)
			r.Get("/reports", h.reports)
		})
	})
	return r
}
func (h *Handler) metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(rw, r)
		h.req.WithLabelValues(r.URL.Path, strconv.Itoa(rw.Status())).Inc()
		h.dur.WithLabelValues(r.URL.Path).Observe(time.Since(start).Seconds())
	})
}
func (h *Handler) uid(r *http.Request) int64 {
	_, c, _ := jwtauth.FromContext(r.Context())
	v, _ := strconv.ParseInt(fmt.Sprint(c["uid"]), 10, 64)
	return v
}
func (h *Handler) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := h.uid(r)
		l := h.lim[uid]
		if l == nil {
			l = rate.NewLimiter(rate.Every(time.Minute/100), 100)
			h.lim[uid] = l
		}
		if !l.Allow() {
			http.Error(w, "rate limit", 429)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func decode(r *http.Request, v any) bool { return json.NewDecoder(r.Body).Decode(v) == nil }
func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Name, Password string }
	if !decode(r, &in) {
		http.Error(w, "bad json", 400)
		return
	}
	id, err := h.s.Register(r.Context(), in.Email, in.Name, in.Password)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	write(w, 201, map[string]int64{"id": id})
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !decode(r, &in) {
		http.Error(w, "bad json", 400)
		return
	}
	u, err := h.s.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	_, token, _ := h.auth.Encode(map[string]any{"uid": u.ID, "exp": time.Now().Add(24 * time.Hour).Unix()})
	write(w, 200, map[string]string{"token": token})
}
func (h *Handler) createTeam(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if !decode(r, &in) {
		http.Error(w, "bad json", 400)
		return
	}
	id, err := h.s.CreateTeam(r.Context(), in.Name, h.uid(r))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	write(w, 201, map[string]int64{"id": id})
}
func (h *Handler) teams(w http.ResponseWriter, r *http.Request) {
	x, err := h.s.Teams(r.Context(), h.uid(r))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	write(w, 200, x)
}
func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		UserID int64       `json:"user_id"`
		Role   domain.Role `json:"role"`
		Email  string      `json:"email"`
	}
	if !decode(r, &in) {
		http.Error(w, "bad json", 400)
		return
	}
	if err := h.s.Invite(r.Context(), h.uid(r), teamID, in.UserID, in.Role, in.Email); err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	write(w, 200, map[string]string{"status": "ok"})
}
func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var t domain.Task
	if !decode(r, &t) {
		http.Error(w, "bad json", 400)
		return
	}
	t.CreatedBy = h.uid(r)
	if t.Status == "" {
		t.Status = domain.StatusTodo
	}
	id, err := h.s.CreateTask(r.Context(), t)
	if err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	write(w, 201, map[string]int64{"id": id})
}
func (h *Handler) tasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	teamID, _ := strconv.ParseInt(q.Get("team_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	var ass *int64
	if q.Get("assignee_id") != "" {
		v, _ := strconv.ParseInt(q.Get("assignee_id"), 10, 64)
		ass = &v
	}
	x, err := h.s.ListTasks(r.Context(), h.uid(r), teamID, q.Get("status"), ass, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	write(w, 200, x)
}
func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var t domain.Task
	if !decode(r, &t) {
		http.Error(w, "bad json", 400)
		return
	}
	if err := h.s.UpdateTask(r.Context(), h.uid(r), id, t); err != nil {
		code := 400
		if strings.Contains(err.Error(), "forbidden") {
			code = 403
		}
		http.Error(w, err.Error(), code)
		return
	}
	write(w, 200, map[string]string{"status": "ok"})
}
func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	x, err := h.s.History(r.Context(), h.uid(r), id)
	if err != nil {
		http.Error(w, err.Error(), 403)
		return
	}
	write(w, 200, x)
}
func (h *Handler) reports(w http.ResponseWriter, r *http.Request) {
	a, b, c, err := h.s.Reports(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	write(w, 200, map[string]any{"team_stats": a, "top_creators": b, "invalid_assignees": c})
}
