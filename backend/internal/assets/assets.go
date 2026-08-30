// Package assets embeds static template content used when generating
// client-facing documents: the LDM.tv logo (extracted from the reference
// delivery note PDF) and the verbatim hire T&Cs boilerplate (see brief §6).
package assets

import _ "embed"

//go:embed ldm_logo.png
var LDMLogoPNG []byte

//go:embed tandc.txt
var TandCText string
