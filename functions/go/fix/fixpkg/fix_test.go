package fixpkg

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/kustomize/kyaml/copyutil"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestFixV1alpha1ToV1(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)
	err = copyutil.CopyDir("../../../../testdata/fix/nginx-v1alpha1", dir)
	assert.NoError(t, err)
	inout := &kio.LocalPackageReadWriter{
		PackagePath:    dir,
		MatchFilesGlob: append(kio.DefaultMatch, "Kptfile"),
	}
	f := &Fix{}
	err = kio.Pipeline{
		Inputs:  []kio.Reader{inout},
		Filters: []kio.Filter{f},
		Outputs: []kio.Writer{inout},
	}.Execute()
	assert.NoError(t, err)
	diff, err := copyutil.Diff(dir, "../../../../testdata/fix/nginx-v1")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(diff.List()))
	results, err := yaml.Marshal(f.Results)
	assert.NoError(t, err)
	assert.Equal(t, `- filepath: Kptfile
  message: Transformed "packageMetadata" to "info"
- filepath: Kptfile
  message: Transformed "upstream" to "upstream" and "upstreamLock"
- filepath: Kptfile
  message: Added "gcr.io/kpt-fn/set-labels:v0.1" to mutators list, please move it to validators section if it is a validator function
- filepath: Kptfile
  message: Transformed "openAPI" definitions to "apply-setters" function
- filepath: hello-world/Kptfile
  message: Transformed "packageMetadata" to "info"
- filepath: hello-world/Kptfile
  message: Transformed "upstream" to "upstream" and "upstreamLock"
- filepath: hello-world/Kptfile
  message: Added "gcr.io/kpt-fn/set-annotations:v0.1" to mutators list, please move it to validators section if it is a validator function
- filepath: hello-world/Kptfile
  message: Added "gcr.io/kpt-fn/set-namespace:v0.1" to mutators list, please move it to validators section if it is a validator function
- filepath: hello-world/Kptfile
  message: Transformed "openAPI" definitions to "apply-setters" function
`, string(results))
}

func TestFixV1alpha2ToV1(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)
	err = copyutil.CopyDir("../../../../testdata/fix/nginx-v1alpha2", dir)
	assert.NoError(t, err)
	inout := &kio.LocalPackageReadWriter{
		PackagePath:    dir,
		MatchFilesGlob: append(kio.DefaultMatch, "Kptfile"),
	}
	f := &Fix{}
	err = kio.Pipeline{
		Inputs:  []kio.Reader{inout},
		Filters: []kio.Filter{f},
		Outputs: []kio.Writer{inout},
	}.Execute()
	assert.NoError(t, err)
	diff, err := copyutil.Diff(dir, "../../../../testdata/fix/nginx-v1")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(diff.List()))
	results, err := yaml.Marshal(f.Results)
	assert.NoError(t, err)
	assert.Equal(t, `- filepath: Kptfile
  message: Updated apiVersion to kpt.dev/v1
- filepath: setters-config.yaml
  message: Moved setters from configMap to configPath
- filepath: hello-world/Kptfile
  message: Updated apiVersion to kpt.dev/v1
- filepath: hello-world/setters-config.yaml
  message: Moved setters from configMap to configPath
`, string(results))
}
