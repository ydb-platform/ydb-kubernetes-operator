package templates

const StorageInitScriptTemplate = `
set -eu

echo DefineBox.txt
cat /opt/kikimr/cfg/DefineBox.txt
/opt/kikimr/bin/kikimr admin bs config invoke --proto-file /opt/kikimr/cfg/DefineBox.txt

echo DefineStoragePools.txt
cat /opt/kikimr/cfg/DefineStoragePools.txt
/opt/kikimr/bin/kikimr admin bs config invoke --proto-file /opt/kikimr/cfg/DefineStoragePools.txt
`
