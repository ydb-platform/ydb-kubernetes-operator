package templates

const StorageInitScriptTemplate = `
set -eu
/opt/kikimr/bin/kikimr admin bs config invoke --proto-file /opt/kikimr/cfg/DefineBox.txt
/opt/kikimr/bin/kikimr admin bs config invoke --proto-file /opt/kikimr/cfg/DefineStoragePools.txt
`
