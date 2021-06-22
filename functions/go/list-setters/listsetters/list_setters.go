package listsetters

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const SetterCommentIdentifier = "# kpt-set: "

var _ kio.Filter = &ListSetters{}

// ListSetters applies the setter values to the resource fields which are tagged
// by the setter reference comments
type ListSetters struct {
	// ScalarSetters holds the user provided values for simple scalar setters
	ScalarSetters []ScalarSetter

	ArraySetters []ArraySetter

	// Results are the results of listing setter values
	Results map[string]*Result

	// filePath file path of resource
	filePath string
}

// ScalarSetter stores name and value of the map setter
type ScalarSetter struct {
	// Name is the name of the setter
	Name string

	// Value is the value of the field to which setter comment is added.
	Value string
}

// ArraySetter stores name and values of the array setter
type ArraySetter struct {
	// Name is the name of the setter
	Name string

	// Values are the values of the field to which setter comment is added.
	Values []string
}

// Result holds result of search and replace operation
type Result struct {
	Name  string
	Value string
	Type  string
	Count int
}

// Filter implements Set as a yaml.Filter
func (as *ListSetters) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if len(as.ScalarSetters) == 0 && len(as.ArraySetters) == 0 {
		return nodes, fmt.Errorf("input setters list cannot be empty")
	}
	for i := range nodes {
		filePath, _, err := kioutil.GetFileAnnotations(nodes[i])
		if err != nil {
			return nodes, err
		}
		as.filePath = filePath
		err = accept(as, nodes[i])
		if err != nil {
			return nil, errors.Wrap(err)
		}
	}
	return nodes, nil
}

func (as *ListSetters) visitMapping(object *yaml.RNode, path string) error {
	return object.VisitFields(func(node *yaml.MapNode) error {
		if node == nil || node.Key.IsNil() || node.Value.IsNil() {
			// don't do IsNilOrEmpty check as empty sequences are allowed
			return nil
		}

		// the aim of this method is to list-setter for sequence nodes
		if node.Value.YNode().Kind != yaml.SequenceNode {
			// return if it is not a sequence node
			return nil
		}

		elements, err := node.Value.Elements()
		if err != nil {
			return errors.Wrap(err)
		}
		// extracts the values in sequence node to an array
		var nodeValues []string
		for _, values := range elements {
			nodeValues = append(nodeValues, values.YNode().Value)
		}
		sort.Strings(nodeValues)

		nodeVal := fmt.Sprint(nodeValues)

		linecomment := node.Key.YNode().LineComment
		if node.Value.YNode().Style == yaml.FlowStyle {
			linecomment = node.Value.YNode().LineComment
		}

		// perform a direct set of the field if it matches
		setterPattern := extractSetterPattern(linecomment)
		if setterPattern == "" {
			// the node is not tagged with setter pattern
			return nil
		}

		setterName := clean(setterPattern)
		if _, ok := as.Results[setterName]; ok {
			if as.Results[setterName].Value == nodeVal {
				as.Results[setterName].Count++
			}
		}

		return nil
	})
}

func (as *ListSetters) visitScalar(object *yaml.RNode, path string) error {
	if object.IsNil() {
		return nil
	}

	if object.YNode().Kind != yaml.ScalarNode {
		// return if it is not a scalar node
		return nil
	}

	linecomment := object.YNode().LineComment

	// perform a direct set of the field if it matches
	setterPattern := extractSetterPattern(linecomment)
	if setterPattern == "" {
		// the node is not tagged with setter pattern
		return nil
	}

	if !shouldSet(setterPattern, as.ScalarSetters) {
		// this means there is no intent from user to modify this setter tagged resources
		return nil
	}

	currentSetterValues := currentSetterValues(setterPattern, object.YNode().Value)
	for setterName, setterValue := range currentSetterValues {
		if _, ok := as.Results[setterName]; ok {
			if as.Results[setterName].Value == setterValue {
				as.Results[setterName].Count++
			}
		}
	}

	return nil
}

// getArraySetter parses the input and returns array setters
func getArraySetter(input *yaml.RNode) []string {
	var output []string

	elements, err := input.Elements()
	if err != nil {
		return output
	}

	for _, as := range elements {
		output = append(output, as.YNode().Value)
	}

	sort.Strings(output)
	return output
}

// Decode decodes the input yaml node into Set struct
func Decode(rn *yaml.RNode, fcd *ListSetters) error {
	fcd.Results = make(map[string]*Result)
	for k, v := range rn.GetDataMap() {
		parsedInput, err := yaml.Parse(v)
		if err != nil {
			return fmt.Errorf("parsing error")
		}
		// checks if the value is SequenceNode
		// adds to the ArraySetters if it is a SequenceNode
		// adds to the ScalarSetters if it is a ScalarNode
		if parsedInput.YNode().Kind == yaml.SequenceNode {
			arrayValues := getArraySetter(parsedInput)
			fcd.ArraySetters = append(fcd.ArraySetters, ArraySetter{Name: k, Values: arrayValues})
			fcd.Results[k] = &Result{Name: k, Value: fmt.Sprint(arrayValues), Count: 0, Type: "list"}
		} else if parsedInput.YNode().Kind == yaml.ScalarNode {
			fcd.ScalarSetters = append(fcd.ScalarSetters, ScalarSetter{Name: k, Value: v})
			fcd.Results[k] = &Result{Name: k, Value: v, Count: 0, Type: fmt.Sprintf("%T", v)}
		}
	}
	return nil
}

func extractSetterPattern(lineComment string) string {
	if !strings.HasPrefix(lineComment, SetterCommentIdentifier) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(lineComment, SetterCommentIdentifier))
}

func shouldSet(pattern string, setters []ScalarSetter) bool {
	for _, s := range setters {
		if strings.Contains(pattern, fmt.Sprintf("${%s}", s.Name)) {
			return true
		}
	}
	return false
}

func currentSetterValues(pattern, value string) map[string]string {
	res := make(map[string]string)
	// get all setter names enclosed in ${}
	// e.g. value: my-app-layer.dev.example.com
	// pattern: my-app-layer.${stage}.${domain}.${tld}
	// urs: [${stage}, ${domain}, ${tld}]
	urs := unresolvedSetters(pattern)
	// and escape pattern
	pattern = regexp.QuoteMeta(pattern)
	// escaped pattern: my-app-layer\.\$\{stage\}\.\$\{domain\}\.\$\{tld\}

	for _, setterName := range urs {
		// escape setter name
		// we need to escape the setterName as well to replace it in the escaped pattern string later
		setterName = regexp.QuoteMeta(setterName)
		pattern = strings.ReplaceAll(
			pattern,
			setterName,
			`(?P<x>.*)`) // x is just a place holder, it could be any alphanumeric string
	}
	// pattern: my-app-layer\.(?P<x>.*)\.(?P<x>.*)\.(?P<x>.*)
	r, err := regexp.Compile(pattern)
	if err != nil {
		// just return empty map if values can't be derived from pattern
		return res
	}
	setterValues := r.FindStringSubmatch(value)
	if len(setterValues) == 0 {
		return res
	}
	// setterValues: [ "my-app-layer.dev.example.com", "dev", "example", "com"]
	setterValues = setterValues[1:]
	// setterValues: [ "dev", "example", "com"]
	if len(urs) != len(setterValues) {
		// just return empty map if values can't be derived
		return res
	}
	for i := range setterValues {
		if setterValues[i] == "" {
			// if any of the value is unresolved return empty map
			// and expect users to provide all values
			return make(map[string]string)
		}
		res[clean(urs[i])] = setterValues[i]
	}
	return res
}

func unresolvedSetters(pattern string) []string {
	re := regexp.MustCompile(`\$\{([^}]*)\}`)
	return re.FindAllString(pattern, -1)
}

// clean extracts value enclosed in ${}
func clean(input string) string {
	input = strings.TrimSpace(input)
	return strings.TrimSuffix(strings.TrimPrefix(input, "${"), "}")
}


func (as *ListSetters) getAns() []*Result{
	var out []*Result
	for _, v := range as.Results {
		out = append(out, v)
	}
	return out
}