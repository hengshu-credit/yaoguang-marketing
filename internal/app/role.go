package app

import "github.com/hengshu-credit/yaoguang-marketing/config"

func (a *App) runs(capability config.RuntimeCapability) bool {
	return a != nil && a.config != nil && a.config.Realtime.Role.Runs(capability)
}
