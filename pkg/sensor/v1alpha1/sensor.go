package v1alpha1

type Sensor struct {
	ApiVersion string
	Kind       string
	Metadata   Metadata
	Spec       Spec
}

type Metadata struct {
	Name      string
	Namespace string
}

type Spec struct {
	Dependencies []*Dependency
	Triggers     []*Trigger
}

type Dependency struct {
	Name            string
	EventName       string
	EventSourceName string
}

type Trigger struct {
	Template    *Template
	AtLeastOnce bool
}

type Template struct {
	Name       string
	Conditions string
	Kafka      interface{}
}
