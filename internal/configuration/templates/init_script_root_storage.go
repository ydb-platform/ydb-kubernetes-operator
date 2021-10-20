package templates

const RootStorageInitScriptTemplate = `
set -eu
/opt/kikimr/bin/kikimr db schema execute /opt/kikimr/cfg/BindRootStorageRequest-Root.txt
`
