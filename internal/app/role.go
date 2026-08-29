package app

import "github.com/Notifuse/notifuse/config"

func (a *App) runs(capability config.RuntimeCapability) bool {
	return a != nil && a.config != nil && a.config.Realtime.Role.Runs(capability)
}
