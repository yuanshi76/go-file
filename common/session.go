package common

import (
	"os"
	"strings"
)

// InitSessionSecret persists the auto-generated session secret to disk so that
// cookie-based sessions survive process restarts. When SESSION_SECRET is set
// via the environment, that value takes precedence and nothing is written.
func InitSessionSecret() {
	if sessionSecretFromEnv {
		return
	}
	if data, err := os.ReadFile(SessionSecretPath); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			SessionSecret = s
			return
		}
	}
	// 0600 so only the owner can read the freshly generated secret.
	if err := os.WriteFile(SessionSecretPath, []byte(SessionSecret), 0600); err != nil {
		SysError("failed to persist session secret: " + err.Error())
	}
}
