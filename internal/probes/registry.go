// Package probes provides the built-in probe registry.
package probes

import (
	"github.com/jandubois/monitor/internal/probe"
	"github.com/jandubois/monitor/internal/probes/command"
	"github.com/jandubois/monitor/internal/probes/debug"
	"github.com/jandubois/monitor/internal/probes/diskspace"
	"github.com/jandubois/monitor/internal/probes/github"
	"github.com/jandubois/monitor/internal/probes/gitstatus"
)

// GetAllDescriptions returns descriptions of all built-in probes.
func GetAllDescriptions() []probe.Description {
	return []probe.Description{
		command.GetDescription(),
		debug.GetDescription(),
		diskspace.GetDescription(),
		github.GetDescription(),
		gitstatus.GetDescription(),
	}
}
