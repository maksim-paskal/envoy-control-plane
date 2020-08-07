/*
Copyright paskal.maksim@gmail.com
Licensed under the Apache License, Version 2.0 (the "License")
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"bytes"
	"encoding/json"
	"text/template"

	"github.com/Masterminds/sprig"
	"gopkg.in/yaml.v2"
)

func convertYAMLtoJSON(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convertYAMLtoJSON(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convertYAMLtoJSON(v)
		}
	}
	return i
}

func getJSONfromYAML(i interface{}) ([]byte, error) {
	yamlObj := convertYAMLtoJSON(i)
	var err error
	var returnJSON []byte

	returnJSON, err = json.Marshal(yamlObj)
	return returnJSON, err
}

func goTemplateFunc(t *template.Template) map[string]interface{} {
	f := sprig.TxtFuncMap()

	f["include"] = func(name string, data interface{}) (string, error) {
		buf := bytes.NewBuffer(nil)
		if err := t.ExecuteTemplate(buf, name, data); err != nil {
			return "", err
		}
		return buf.String(), nil
	}

	f["toYaml"] = func(v interface{}) string {
		data, err := yaml.Marshal(v)
		if err != nil {
			// Swallow errors inside of a template.
			return ""
		}
		return string(data)
	}

	return f
}
