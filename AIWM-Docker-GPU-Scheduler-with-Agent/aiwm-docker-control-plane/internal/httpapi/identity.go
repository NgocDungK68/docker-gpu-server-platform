package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// sessionAuth xác thực mỗi request từ PostgreSQL; không fallback token dùng chung.
func sessionAuth(identity *application.IdentityService,next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
		if !strings.HasPrefix(r.URL.Path,"/api/v1/") || strings.HasPrefix(r.URL.Path,"/api/v1/agents/") || strings.HasPrefix(r.URL.Path,"/api/v1/training/") || r.URL.Path=="/api/v1/auth/login" {
			next.ServeHTTP(w,r); return
		}
		p,err:=identity.Authenticate(r.Context(),bearerToken(r.Header.Get("Authorization")))
		if err!=nil { respond(w,nil,err,0); return }
		if filter:=r.URL.Query().Get("organizationId"); filter!="" {
			if p.User.Role!=domain.RoleAdmin { respond(w,nil,domain.ErrForbidden,0); return }
			p.ScopeOrganizationID=filter
		}
		if r.URL.Path=="/api/v1/scheduler/run-once" && p.User.Role!=domain.RoleAdmin { respond(w,nil,domain.ErrForbidden,0); return }
		w.Header().Set("Cache-Control","no-store")
		next.ServeHTTP(w,r.WithContext(domain.WithPrincipal(r.Context(),p)))
	})
}

func mountIdentity(mux *http.ServeMux,s *application.IdentityService) {
	// Giới hạn CPU hashing và số lần login theo địa chỉ peer; không tin X-Forwarded-For.
	var mu sync.Mutex
	type window struct { at time.Time; count int }
	windows:=map[string]window{}
	slots:=make(chan struct{},4)
	mux.HandleFunc("POST /api/v1/auth/login",func(w http.ResponseWriter,r *http.Request) {
		peer,_,_:=net.SplitHostPort(r.RemoteAddr)
		now:=time.Now(); mu.Lock()
		for key,v:=range windows { if now.Sub(v.at)>time.Minute { delete(windows,key) } }
		v:=windows[peer]; if v.at.IsZero() { v.at=now }
		allowed:=v.count<20 && len(windows)<4096
		if allowed { v.count++; windows[peer]=v }; mu.Unlock()
		if !allowed { w.Header().Set("Retry-After","60"); writeError(w,429,"RATE_LIMIT","Vui lòng thử lại sau một phút"); return }
		select { case slots<-struct{}{}: defer func(){ <-slots }(); default: writeError(w,429,"RATE_LIMIT","Có nhiều yêu cầu đăng nhập, vui lòng thử lại"); return }
		var input struct { Username string `json:"username"`; Password string `json:"password"` }
		if err:=decodeJSON(r,&input); err!=nil { respond(w,nil,err,0); return }
		result,err:=s.Login(r.Context(),input.Username,input.Password)
		w.Header().Set("Cache-Control","no-store")
		respond(w,result,err,http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/auth/me",func(w http.ResponseWriter,r *http.Request) {
		p,_:=domain.CurrentPrincipal(r.Context()); respond(w,p,nil,200)
	})
	mux.HandleFunc("POST /api/v1/auth/logout",func(w http.ResponseWriter,r *http.Request) {
		err:=s.Logout(r.Context(),bearerToken(r.Header.Get("Authorization"))); respond(w,map[string]bool{"loggedOut":err==nil},err,200)
	})
	mux.HandleFunc("GET /api/v1/organizations",func(w http.ResponseWriter,r *http.Request) {
		items,err:=s.Organizations(r.Context()); respond(w,items,err,200)
	})
	saveOrganization:=func(w http.ResponseWriter,r *http.Request) {
		var input application.OrganizationInput
		if err:=decodeJSON(r,&input); err!=nil { respond(w,nil,err,0); return }
		item,err:=s.SaveOrganization(r.Context(),r.PathValue("organizationID"),input); respond(w,item,err,200)
	}
	mux.HandleFunc("POST /api/v1/organizations",saveOrganization)
	mux.HandleFunc("POST /api/v1/organizations/{organizationID}",saveOrganization)
	mux.HandleFunc("GET /api/v1/users",func(w http.ResponseWriter,r *http.Request) {
		items,err:=s.Users(r.Context()); respond(w,items,err,200)
	})
	saveUser:=func(w http.ResponseWriter,r *http.Request) {
		var input application.UserInput
		if err:=decodeJSON(r,&input); err!=nil { respond(w,nil,err,0); return }
		item,err:=s.SaveUser(r.Context(),r.PathValue("userID"),input); respond(w,item,err,200)
	}
	mux.HandleFunc("POST /api/v1/users",saveUser)
	mux.HandleFunc("POST /api/v1/users/{userID}",saveUser)
	mux.HandleFunc("POST /api/v1/enrollments",func(w http.ResponseWriter,r *http.Request) {
		var input application.EnrollmentInput
		if err:=decodeJSON(r,&input); err!=nil { respond(w,nil,err,0); return }
		result,err:=s.CreateEnrollment(r.Context(),input); respond(w,result,err,201)
	})
	mux.HandleFunc("GET /api/v1/enrollments",func(w http.ResponseWriter,r *http.Request) {
		items,err:=s.Enrollments(r.Context()); respond(w,items,err,200)
	})
	mux.HandleFunc("POST /api/v1/enrollments/{enrollmentID}/revoke",func(w http.ResponseWriter,r *http.Request) {
		err:=s.RevokeEnrollment(r.Context(),r.PathValue("enrollmentID")); respond(w,map[string]bool{"revoked":err==nil},err,200)
	})
}
