

// Code generated by "mdtogo"; DO NOT EDIT.
package generated

var GatekeeperShort = `Validate the KRM resources using [Gatekeeper] constraints.`
var GatekeeperLong = `
[Gatekeeper] allows users to validate the KRM resources against the Gatekeeper
constraints.

You will need to define a [Constraint Template] first before defining a
[Constraint]. Every constraint should be backed by a constraint template that
defines the schema and logic of the constraint.
To learn more about how to use the Gatekeeper project, see [here].

At least one constraint template and at least one constraint must be provided
using ` + "`" + `resourceList.items` + "`" + ` along with other KRM resources. No function config is
needed in ` + "`" + `resourceList.functionConfig` + "`" + `.
`