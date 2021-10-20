package templates

const ComputeInitScriptTemplate = `
set -eu
/opt/kikimr/bin/kikimr db schema init Root
`
