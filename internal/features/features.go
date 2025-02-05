package features

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

const (
	// Add new feature gates constants (strings)
	// Ex: SomeFeature featuregate.Feature = "SomeFeature"
	PreflightPermissions featuregate.Feature = "PreflightPermissions"
	APIV1QueryHandler                        = featuregate.Feature("APIV1QueryHandler")
)

var catalogdFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	APIV1QueryHandler: {Default: false, PreRelease: featuregate.Alpha},
}

var operatorControllerFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	// Add new feature gate definitions
	// Ex: SomeFeature: {...}
	PreflightPermissions: {
		Default:       false,
		PreRelease:    featuregate.Alpha,
		LockToDefault: false,
	},
}

var OperatorControllerFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

var CatalogdFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

func init() {
	utilruntime.Must(OperatorControllerFeatureGate.Add(operatorControllerFeatureGates))
	utilruntime.Must(CatalogdFeatureGate.Add(catalogdFeatureGates))

}
