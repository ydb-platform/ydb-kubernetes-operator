package configuration

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/templates"
)

var templateToFilename = map[string]string{
	"auth.txt":          templates.AuthTemplate,
	"boot.txt":          templates.BootTemplate,
	"bs.txt":            templates.BlobStoreTemplate,
	"channels.txt":      templates.ChannelsTemplate,
	"names.txt":         templates.NamesTemplate,
	"domains.txt":       templates.DomainTemplate,
	"DefineBox.txt":     templates.DefineBoxTemplate,
	"feature_flags.txt": templates.FeatureFlagTemplate,
	"grpc.txt":          templates.GrpcTemplate,
	"ic.txt":            templates.InterconnectTemplate,
	"kqp.txt":           templates.KqpTemplate,
	"log.txt":           templates.LogTemplate,
	"pq.txt":            templates.PqTemplate,
	"sys.txt":           templates.SysTemplate,
	"vdisks.txt":        templates.VdiskTemplate,
}

type MapWrapper struct {
	Amap map[string]string
}

type ClusterObjectWrapper struct {
	*v1alpha1.Storage

	GRPCPort         int
	InterconnectPort int
	StatusPort       int
}

var additionalFuncs = template.FuncMap{
	"iter": func(count int32) []int32 {
		var i int32
		var items []int32
		for i = 0; i < (count); i++ {
			items = append(items, i)
		}
		return items
	},
	"add": func(a int32, b int32) int32 {
		return a + b
	},

	"indent": func(spaces int, v string) string {
		pad := strings.Repeat(" ", spaces)
		return pad + strings.Replace(v, "\n", "\n"+pad, -1)
	},

	"hasKey": func(d map[string]string, key string) bool {
		_, ok := d[key]
		return ok
	},
}

func Build(cr *v1alpha1.Storage) (map[string]string, error) {
	var err error

	result := make(map[string]string)

	templateData := ClusterObjectWrapper{
		Storage:          cr,
		GRPCPort:         v1alpha1.GRPCPort,
		InterconnectPort: v1alpha1.InterconnectPort,
		StatusPort:       v1alpha1.StatusPort,
	}

	for filename, templateText := range templateToFilename {
		if result[filename], err = applyTemplate(templateText, templateData); err != nil {
			return nil, err
		}
	}

	for k, v := range cr.Spec.ClusterConfig {
		result[k] = v
	}

	if _, ok := result["Configure-Root.txt"]; !ok {
		configureRoot, err := applyTemplate(templates.ConfigureRootTemplate, MapWrapper{
			Amap: result,
		})
		if err != nil {
			return nil, err
		}
		result["Configure-Root.txt"] = configureRoot

	}

	return result, nil
}

func applyTemplate(templateText string, data interface{}) (string, error) {
	buffer := &bytes.Buffer{}

	tpl := template.Must(
		template.New("").Funcs(additionalFuncs).Parse(templateText),
	)

	err := tpl.Execute(buffer, data)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}
