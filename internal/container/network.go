package container

import "github.com/r-dson/abox/internal/config"

// ResolveNetworkMode determines the Docker network mode from config and session state.
func ResolveNetworkMode(sess *Session, cfg *config.Config) string {
	if cfg.NoInternet {
		return "none"
	}
	if sess.NetworkID() != "" {
		return sess.NetworkID()
	}
	return "" // default bridge
}
