package createsetters

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/kustomize/kyaml/kio"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestCreateSettersFilter(t *testing.T) {
	var tests = []struct {
		name              string
		config            string
		input             string
		expectedResources string
		errMsg            string
	}{
		{
			name: "apply array setter for flow style",
			config: `
data:
  env: |
    [foo, bar]
  name: nginx
`,
			input: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  env: [foo, bar]
 `,
			expectedResources: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment # kpt-set: ${name}-deployment
  env: [foo, bar] # kpt-set: ${env}
`,

		},
		{
			name: "set comment to Setters and ArraySetters",
			input: `apiVersion: v1
kind: Service
metadata:
  name: myService
  namespace: foo 
image: nginx:1.7.1 
env: [foo, bar] 
roles:
  - dev
  - prod
`,
			config: `
data:
  app: myService
  ns: foo
  image: nginx
  tag: 1.7.1
  env: "[foo, bar]"
  roles: |
    - prod
    - dev
`,
			expectedResources: `apiVersion: v1
kind: Service
metadata:
  name: myService # kpt-set: ${app}
  namespace: foo # kpt-set: ${ns}
image: nginx:1.7.1 # kpt-set: ${image}:${tag}
env: [foo, bar] # kpt-set: ${env}
roles: # kpt-set: ${roles}
  - dev
  - prod
`,
		},
		
	}
	for i := range tests {
		test := tests[i]
		t.Run(test.name, func(t *testing.T) {
			baseDir, err := ioutil.TempDir("", "")
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			defer os.RemoveAll(baseDir)

			r, err := ioutil.TempFile(baseDir, "k8s-cli-*.yaml")
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			defer os.Remove(r.Name())
			err = ioutil.WriteFile(r.Name(), []byte(test.input), 0600)
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			s := &CreateSetters{}
			node, err := kyaml.Parse(test.config)
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			Decode(node, s)
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			inout := &kio.LocalPackageReadWriter{
				PackagePath:     baseDir,
				NoDeleteFiles:   true,
				PackageFileName: "Kptfile",
			}
			err = kio.Pipeline{
				Inputs:  []kio.Reader{inout},
				Filters: []kio.Filter{s},
				Outputs: []kio.Writer{inout},
			}.Execute()
			if test.errMsg != "" {
				if !assert.NotNil(t, err) {
					t.FailNow()
				}
				if !assert.Contains(t, err.Error(), test.errMsg) {
					t.FailNow()
				}
			}

			if test.errMsg == "" && !assert.NoError(t, err) {
				t.FailNow()
			}

			actualResources, err := ioutil.ReadFile(r.Name())
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			if !assert.Equal(t,
				test.expectedResources,
				string(actualResources)) {
				t.FailNow()
			}
		})
	}
}

type patternTest struct {
	name     string
	value    string
	pattern  string
	expected map[string]string
}

var resolvePatternCases = []patternTest{
	{
		name:    "setter values from pattern 1",
		value:   "foo-dev-bar-us-east-1-baz",
		pattern: `foo-${environment}-bar-${region}-baz`,
		expected: map[string]string{
			"environment": "dev",
			"region":      "us-east-1",
		},
	},
	{
		name:    "setter values from pattern 2",
		value:   "foo-dev-bar-us-east-1-baz",
		pattern: `foo-${environment}-bar-${region}-baz`,
		expected: map[string]string{
			"environment": "dev",
			"region":      "us-east-1",
		},
	},
	{
		name:    "setter values from pattern 3",
		value:   "gcr.io/my-app/my-app-backend:1.0.0",
		pattern: `${registry}/${app~!@#$%^&*()<>?:"|}/${app-image-name}:${app-image-tag}`,
		expected: map[string]string{
			"registry":             "gcr.io",
			`app~!@#$%^&*()<>?:"|`: "my-app",
			"app-image-name":       "my-app-backend",
			"app-image-tag":        "1.0.0",
		},
	},
	{
		name:     "setter values from pattern unresolved",
		value:    "foo-dev-bar-us-east-1-baz",
		pattern:  `${image}:${tag}`,
		expected: map[string]string{},
	},
	{
		name:     "setter values from pattern unresolved 2",
		value:    "nginx:1.2",
		pattern:  `${image}${tag}`,
		expected: map[string]string{},
	},
	{
		name:     "setter values from pattern unresolved 3",
		value:    "my-project/nginx:1.2",
		pattern:  `${project-id}/${image}${tag}`,
		expected: map[string]string{},
	},
}


