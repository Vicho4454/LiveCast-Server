package mediamtx

import "github.com/bluenviron/mediamtx/internal/core"

// Server struct maps the internal MediaMTX core
type Server struct {
	core *core.Core
}

// Start initializes the MediaMTX core bypassing the internal restriction
func Start() (*Server, bool) {
	c, ok := core.New([]string{})
	if !ok {
		return nil, false
	}
	return &Server{core: c}, true
}

// Wait blocks until the server closes
func (s *Server) Wait() {
	s.core.Wait()
}

// Close gracefully stops the server
func (s *Server) Close() {
	s.core.Close()
}
