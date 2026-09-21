package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// ConsoleThemeSettings styles only the authenticated console, never message content.
type ConsoleThemeSettings struct {
	Version        int      `json:"version"`
	ColorScheme    string   `json:"color_scheme"`
	AccentMode     string   `json:"accent_mode"`
	AccentPreset   string   `json:"accent_preset"`
	PrimaryColor   string   `json:"primary_color"`
	GradientColors []string `json:"gradient_colors"`
	GradientType   string   `json:"gradient_type"`
	GradientAngle  *int     `json:"gradient_angle"`
	SidebarStyle   string   `json:"sidebar_style"`
	ContentWidth   string   `json:"content_width"`
	Density        string   `json:"density"`
	TitleStyle     string   `json:"title_style"`
}

var consoleColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func DefaultConsoleTheme() ConsoleThemeSettings {
	angle := 135
	return ConsoleThemeSettings{Version: 1, ColorScheme: "light", AccentMode: "preset", AccentPreset: "yaoguang", PrimaryColor: "#7763F1", GradientColors: []string{"#2639E9", "#873FF2"}, GradientType: "linear", GradientAngle: &angle, SidebarStyle: "light", ContentWidth: "fluid", Density: "comfortable", TitleStyle: "accent"}
}

func (c *ConsoleThemeSettings) Validate() error {
	if c == nil {
		return nil
	}
	defaults := DefaultConsoleTheme()
	if c.Version == 0 {
		c.Version = defaults.Version
	}
	if c.Version != 1 {
		return fmt.Errorf("console_theme.version must be 1")
	}
	for _, entry := range []struct {
		field    string
		value    *string
		fallback string
		allowed  []string
	}{
		{"color_scheme", &c.ColorScheme, defaults.ColorScheme, []string{"light", "dark", "system"}},
		{"accent_mode", &c.AccentMode, defaults.AccentMode, []string{"preset", "solid", "gradient"}},
		{"accent_preset", &c.AccentPreset, defaults.AccentPreset, []string{"yaoguang", "blue", "purple", "teal", "rose", "indigo", "nebula", "dusk"}},
		{"gradient_type", &c.GradientType, defaults.GradientType, []string{"linear", "radial"}},
		{"sidebar_style", &c.SidebarStyle, defaults.SidebarStyle, []string{"light", "dark"}},
		{"content_width", &c.ContentWidth, defaults.ContentWidth, []string{"fluid", "fixed"}},
		{"density", &c.Density, defaults.Density, []string{"comfortable", "compact"}},
		{"title_style", &c.TitleStyle, defaults.TitleStyle, []string{"accent", "plain", "compact"}},
	} {
		if *entry.value == "" {
			*entry.value = entry.fallback
		}
		valid := false
		for _, allowed := range entry.allowed {
			if *entry.value == allowed {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("console_theme.%s is invalid", entry.field)
		}
	}
	if c.PrimaryColor == "" {
		c.PrimaryColor = defaults.PrimaryColor
	}
	if !consoleColorPattern.MatchString(c.PrimaryColor) {
		return fmt.Errorf("console_theme.primary_color must be a six-digit hex color")
	}
	c.PrimaryColor = strings.ToUpper(c.PrimaryColor)
	if c.GradientColors == nil {
		c.GradientColors = defaults.GradientColors
	}
	if c.GradientAngle == nil {
		c.GradientAngle = defaults.GradientAngle
	}
	if len(c.GradientColors) < 2 || len(c.GradientColors) > 3 {
		return fmt.Errorf("console_theme.gradient_colors requires two or three colors")
	}
	for index, color := range c.GradientColors {
		if !consoleColorPattern.MatchString(color) {
			return fmt.Errorf("console_theme.gradient_colors must contain six-digit hex colors")
		}
		c.GradientColors[index] = strings.ToUpper(color)
	}
	if *c.GradientAngle < 0 || *c.GradientAngle > 360 {
		return fmt.Errorf("console_theme.gradient_angle must be between 0 and 360")
	}
	return nil
}
