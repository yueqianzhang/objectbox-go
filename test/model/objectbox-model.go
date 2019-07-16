// Code generated by ObjectBox; DO NOT EDIT.

package model

import (
	"github.com/objectbox/objectbox-go/objectbox"
)

// ObjectBoxModel declares and builds the model from all the entities in the package.
// It is usually used when setting-up ObjectBox as an argument to the Builder.Model() function.
func ObjectBoxModel() *objectbox.Model {
	model := objectbox.NewModel()
	model.GeneratorVersion(3)

	model.RegisterBinding(EntityBinding)
	model.RegisterBinding(TestStringIdEntityBinding)
	model.RegisterBinding(EntityByValueBinding)
	model.RegisterBinding(TestEntityInlineBinding)
	model.RegisterBinding(TestEntityRelatedBinding)
	model.LastEntityId(5, 145948658381494339)
	model.LastIndexId(4, 3414034888235702623)
	model.LastRelationId(6, 3119566795324383223)

	return model
}
