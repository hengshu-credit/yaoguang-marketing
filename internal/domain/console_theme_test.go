package domain

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConsoleThemeValidationAndWorkspaceRoundTrip(t *testing.T) {
	var empty *ConsoleThemeSettings
	require.NoError(t, empty.Validate())
	value := DefaultConsoleTheme()
	value.PrimaryColor = "#aabbcc"
	*value.GradientAngle = 0
	require.NoError(t, value.Validate())
	require.Equal(t, "#AABBCC", value.PrimaryColor)
	require.Zero(t, *value.GradientAngle)
	var partial ConsoleThemeSettings
	require.NoError(t, json.Unmarshal([]byte(`{"gradient_angle":0}`), &partial))
	require.NoError(t, partial.Validate())
	require.Zero(t, *partial.GradientAngle)
	settings := WorkspaceSettings{ConsoleTheme: &value, ConsoleFont: &ConsoleFontSettings{Family: "Arial"}}
	raw, err := json.Marshal(settings)
	require.NoError(t, err)
	var decoded WorkspaceSettings
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, settings.ConsoleTheme, decoded.ConsoleTheme)
	require.Equal(t, settings.ConsoleFont, decoded.ConsoleFont)
	for _, mutate := range []func(*ConsoleThemeSettings){
		func(v *ConsoleThemeSettings) { v.Version = 99 }, func(v *ConsoleThemeSettings) { v.PrimaryColor = "url(javascript:bad)" },
		func(v *ConsoleThemeSettings) { v.GradientColors = []string{"red", "blue"} }, func(v *ConsoleThemeSettings) { v.GradientColors = []string{} },
		func(v *ConsoleThemeSettings) { *v.GradientAngle = 361 }, func(v *ConsoleThemeSettings) { v.SidebarStyle = "auto" },
		func(v *ConsoleThemeSettings) { v.AccentPreset = "unknown" }, func(v *ConsoleThemeSettings) { v.TitleStyle = "hidden" },
	} {
		v := DefaultConsoleTheme()
		mutate(&v)
		require.Error(t, v.Validate())
	}
}
