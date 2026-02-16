package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// registerLLMSTxtRoutes adds llms.txt and docs routes.
func (s *Server) registerLLMSTxtRoutes() {
	if s.llmsTxt == "" {
		return
	}

	// Serve llms.txt at /.well-known/llms.txt
	s.mux.HandleFunc("/.well-known/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(s.llmsTxt))
	})

	// Serve also at /llms.txt for convenience
	s.mux.HandleFunc("/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(s.llmsTxt))
	})

	// Serve docs as markdown
	if s.docsFS == nil {
		return
	}
	s.mux.HandleFunc("/v1/docs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/docs/")
		if name == "" || !strings.HasSuffix(name, ".md") {
			http.NotFound(w, r)
			return
		}
		content, err := fs.ReadFile(s.docsFS, "docs/"+name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Write(content)
	})
}
