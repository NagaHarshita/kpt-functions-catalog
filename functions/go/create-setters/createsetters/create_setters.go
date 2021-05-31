package createsetters

import (
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var _ kio.Filter = &CreateSetters{}

// CreateSetters creates a comment for the resource fields which
// contains the same value as setter reference value
type CreateSetters struct {
	// Setters holds the user provided values for simple map setters
	Setters []Setter

	// Setters holds the user provided values for array setters
	ArraySetters []ArraySetter

	// Results are the results of applying setter values
	Results []*Result

	// filePath file path of resource
	filePath string
}

// Setter stores name and value of the map setter
type Setter struct {
	// Name is the name of the setter
	Name string

	// Value is the input value for setter
	Value string
}

// ArraySetter stores name and values of the array setter
type ArraySetter struct {
	// Name is the name of the setter
	Name string

	// Values is the set of the values for setter
	Values []string
}

// Result holds result of create-setters operation
type Result struct {
	// FilePath is the file path of the matching value
	FilePath string

	// FieldPath is field path of the matching value
	FieldPath string

	// Value of the matching value
	Value string

	// LineComment of the matching value
	Comment string
}

type CompareSetters []Setter

func (a CompareSetters) Len() int {
	return len(a)
}

func (a CompareSetters) Less(i, j int) bool {
	if strings.Contains(a[i].Value, a[j].Name) {
		return false
	}
	return true
}

func (a CompareSetters) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// Filter implements CreatSetters cs a yaml.Filter
func (cs *CreateSetters) Filter(nodes []*yaml.RNode) ([]*yaml.RNode, error) {
	if len(cs.Setters) == 0 {
		return nodes, fmt.Errorf("input setters list cannot be empty")
	}
	for i := range nodes {
		filePath, _, err := kioutil.GetFileAnnotations(nodes[i])
		if err != nil {
			return nodes, err
		}
		cs.filePath = filePath
		err = accept(cs, nodes[i])
		if err != nil {
			return nil, errors.Wrap(err)
		}
	}
	return nodes, nil
}

/**
visitMapping takes the mapping node and performs following steps,
checks if it is a sequence node
checks if all the values in the node are present to any of the ArraySetters
adds the linecomment if they are equal

e.g. for input of Mapping node

environments:
  - dev
  - stage

For input CreateSetters [Name: env, Values: [dev, stage]], yaml node is transformed to

environments: # kpt-set: ${env}
  - dev
  - stage

e.g. for input of Mapping node with FlowStyle

env: [foo, bar]

For input CreateSetters [Name: env, Values: [foo, bar]], yaml node is transformed to

env: [foo, bar] # kpt-set: ${env}
*/

func (cs *CreateSetters) visitMapping(object *yaml.RNode, path string) error {
	return object.VisitFields(func(node *yaml.MapNode) error {
		if node == nil || node.Key.IsNil() || node.Value.IsNil() {
			// don't do IsNilOrEmpty check cs empty sequences are allowed
			return nil
		}

		// the aim of this method is to create-setter for sequence nodes
		if node.Value.YNode().Kind != yaml.SequenceNode {
			// return if it is not a sequence node
			return nil
		}		

		// add the key to the field path
		fieldPath := strings.TrimPrefix(fmt.Sprintf("%s.%s", path, node.Key.YNode().Value), ".")

		elements, err := node.Value.Elements()
		if err != nil {
			return errors.Wrap(err)
		}
		// extracts the values in sequence node to an array
		var nodeValues []string
		for _, values := range elements {
			nodeValues = append(nodeValues, values.YNode().Value)
		}

		// checks if the kind is flowstyle and adds comment to its value node
		// else it adds the comment to the key node
		nodeToAddComment := node.Value
		if nodeToAddComment.YNode().Style == yaml.FlowStyle {
			if hasMatchValue(nodeValues, cs.Setters) {
				// changing the node style to FoldedStyle
				nodeToAddComment.YNode().Style = yaml.FoldedStyle
				// To add the comment to the key for the FoldedStyle value node
				nodeToAddComment = node.Key
			}
		}

		for _, arraySetters := range cs.ArraySetters {
			// checks if all the values in node are present in array setter
			if checkEqual(nodeValues, arraySetters.Values) {
				nodeToAddComment.YNode().LineComment = fmt.Sprintf("kpt-set: ${%s}", arraySetters.Name)
				return nil
			}
		}

		cs.Results = append(cs.Results, &Result{
			FilePath:  cs.filePath,
			FieldPath: fieldPath,
			Value:     nodeToAddComment.YNode().Value,
			Comment:   nodeToAddComment.YNode().LineComment,
		})
		return nil
	})
}

/**
visitScalar accepts the input scalar node and performs following steps,
checks if it is a scalar node
adds the linecomment if it's value matches with any of the setter

e.g.for input of scalar node 'nginx:1.7.1' in the yaml node

apiVersion: v1
...
  image: nginx:1.7.1

and for input CreateSetters [[name: image, value: nginx], [name: tag, value: 1.7.1]]
The yaml node is transformed to

apiVersion: v1
...
  image: nginx:1.7.1 # kpt-set: ${image}:${tag}

*/

func (cs *CreateSetters) visitScalar(object *yaml.RNode, path string) error {
	if object.YNode().Kind != yaml.ScalarNode {
		// return if it is not a scalar node
		return nil
	}

	linecomment, valueMatch := getLineComment(object.YNode().Value, cs.Setters)

	// sets the linecomment
	if valueMatch {
		object.YNode().LineComment = fmt.Sprintf("kpt-set: %s", linecomment)
	}

	cs.Results = append(cs.Results, &Result{
		FilePath:  cs.filePath,
		FieldPath: strings.TrimPrefix(path, "."),
		Value:     object.YNode().Value,
		Comment:   object.YNode().LineComment,
	})

	return nil
}

// Decode decodes the input yaml node into CreatSetters struct
func Decode(rn *yaml.RNode, fcd *CreateSetters) error {
	for k, v := range rn.GetDataMap() {
		// add the setter to ArraySetters if value is array
		// else add to the Setters
		parsedInput, err := yaml.Parse(v)
		if err != nil {
			return err
		}
		if parsedInput.YNode().Kind == yaml.SequenceNode {
			fcd.ArraySetters = append(fcd.ArraySetters, ArraySetter{Name: k, Values: getArraySetter(parsedInput)})
		} else if parsedInput.YNode().Kind == yaml.ScalarNode {
			fcd.Setters = append(fcd.Setters, Setter{Name: k, Value: v})
			sort.Sort(CompareSetters(fcd.Setters))
		}
	}
	return nil
}

// checkEqual checks if all the values in node are present in array setter
func checkEqual(nodeValues []string, arraySetters []string) bool {
	if len(nodeValues) != len(arraySetters) {
		return false
	}

	sort.Strings(nodeValues)
	for idx := range nodeValues {
		if arraySetters[idx] != nodeValues[idx] {
			return false
		}
	}
	return true
}

// parses the input and returns array setters
func getArraySetter(input *yaml.RNode) []string {
	output := []string{}

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

// checkEqual checks if all the values in node are present in array setter
func hasMatchValue(nodeValues []string, setters []Setter) bool {
	for _, value := range nodeValues {
		for _, setter := range setters {
			if strings.Contains(value, setter.Value) {
				return true
			}
		}
	}
	return false
}

func getLineComment(input string, setters []Setter) (string, bool) {
	nodeValue := input
	output := input
	valueMatch := false

	for _, setter := range setters {
		if strings.Contains(nodeValue, setter.Value) {
			valueMatch = true
			output = strings.ReplaceAll(
				output,
				setter.Value,
				fmt.Sprintf("${%s}", setter.Name),
			)
		}
	}

	return output, valueMatch
}
