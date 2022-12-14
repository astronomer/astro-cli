package include

import _ "embed"

//go:embed dockerignore
var Dockerignore string

//go:embed dockerfile
var Dockerfile string

//go:embed gitignore
var Gitignore string

//go:embed settingsyml.yml
var Settingsyml string

//go:embed advancedexampledag.py
var ExampleDagAdvanced string

//go:embed basicexampledag.py
var ExampleDagBasic string

//go:embed dagintegritytest.py
var DagIntegrityTest string

//go:embed dagintegritytestdefault.py
var DagIntegrityTestDefault string

//go:embed readme
var Readme string

//go:embed composeyml.yml
var Composeyml string

//go:embed astronomermonitoringdag.py
var MonitoringDag string
