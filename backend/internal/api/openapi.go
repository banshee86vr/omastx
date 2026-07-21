package api

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"sigs.k8s.io/yaml"
)

//go:embed openapi.yaml
var openapiYAML []byte

func (s *Server) handleOpenAPIYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiYAML)
}

func (s *Server) handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	var doc any
	if err := yaml.Unmarshal(openapiYAML, &doc); err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(doc)
}
