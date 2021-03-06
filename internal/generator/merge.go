/*
 * Copyright 2019 ObjectBox Ltd. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package generator

import (
	"fmt"
	"log"

	"github.com/objectbox/objectbox-go/internal/generator/modelinfo"
)

func mergeBindingWithModelInfo(binding *Binding, modelInfo *modelinfo.ModelInfo) error {
	// we need to first prepare all entities - otherwise relations wouldn't be able to find them in the model
	var models = make([]*modelinfo.Entity, len(binding.Entities))
	var err error
	for k, bindingEntity := range binding.Entities {
		models[k], err = getModelEntity(bindingEntity, modelInfo)
		if err != nil {
			return err
		}
	}

	for k, bindingEntity := range binding.Entities {
		if err := mergeModelEntity(bindingEntity, models[k], modelInfo); err != nil {
			return err
		}
	}

	// NOTE this is not ideal as there could be models across multiple packages
	modelInfo.Package = binding.Package.Name()

	return nil
}

func getModelEntity(bindingEntity *Entity, modelInfo *modelinfo.ModelInfo) (*modelinfo.Entity, error) {
	if bindingEntity.Uid != 0 {
		return modelInfo.FindEntityByUid(bindingEntity.Uid)
	}

	// we don't care about this error = either the entity is found or we create it
	entity, _ := modelInfo.FindEntityByName(bindingEntity.Name)

	// handle uid request
	if bindingEntity.uidRequest {
		var errInfo string
		if entity != nil {
			uid, err := entity.Id.GetUid()
			if err != nil {
				return nil, err
			}
			errInfo = fmt.Sprintf("model entity UID = %d", uid)
		} else {
			errInfo = "entity not found in the model"
		}
		return nil, fmt.Errorf("uid annotation value must not be empty (%s) on entity %s", errInfo, bindingEntity.Name)
	}

	if entity == nil {
		return modelInfo.CreateEntity(bindingEntity.Name)
	}

	return entity, nil
}

func mergeModelEntity(bindingEntity *Entity, modelEntity *modelinfo.Entity, modelInfo *modelinfo.ModelInfo) (err error) {
	modelEntity.Name = bindingEntity.Name

	if bindingEntity.Id, bindingEntity.Uid, err = modelEntity.Id.Get(); err != nil {
		return err
	}

	{ //region Properties

		// add all properties from the bindings to the model and update/rename the changed ones
		for _, bindingProperty := range bindingEntity.Properties {
			if modelProperty, err := getModelProperty(bindingProperty, modelEntity, modelInfo); err != nil {
				return err
			} else if err := mergeModelProperty(bindingProperty, modelProperty); err != nil {
				return err
			}
		}

		// remove the missing (removed) properties
		removedProperties := make([]*modelinfo.Property, 0)
		for _, modelProperty := range modelEntity.Properties {
			if !bindingPropertyExists(modelProperty, bindingEntity) {
				removedProperties = append(removedProperties, modelProperty)
			}
		}

		for _, property := range removedProperties {
			if err := modelEntity.RemoveProperty(property); err != nil {
				return err
			}
		}

		bindingEntity.LastPropertyId = modelEntity.LastPropertyId
	} //endregion

	{ //region Relations

		// add all standalone relations from the bindings to the model and update/rename the changed ones
		for _, bindingRelation := range bindingEntity.Relations {
			if modelRelation, err := getModelRelation(bindingRelation, modelEntity); err != nil {
				return err
			} else if err := mergeModelRelation(bindingRelation, modelRelation, modelInfo); err != nil {
				return err
			}
		}

		// remove the missing (removed) relations
		removedRelations := make([]*modelinfo.StandaloneRelation, 0)
		for _, modelRelation := range modelEntity.Relations {
			if !bindingRelationExists(modelRelation, bindingEntity) {
				removedRelations = append(removedRelations, modelRelation)
			}
		}

		for _, relation := range removedRelations {
			if err := modelEntity.RemoveRelation(relation); err != nil {
				return err
			}
		}
	} //endregion

	return nil
}

func getModelProperty(bindingProperty *Property, modelEntity *modelinfo.Entity, modelInfo *modelinfo.ModelInfo) (*modelinfo.Property, error) {
	if bindingProperty.Uid != 0 {
		property, err := modelEntity.FindPropertyByUid(bindingProperty.Uid)
		if err == nil {
			return property, nil
		}

		// handle "reset property data" use-case - adding a new UID to an existing property
		property, err2 := modelEntity.FindPropertyByName(bindingProperty.Name)
		if err2 != nil {
			return nil, fmt.Errorf("%v; %v", err, err2)
		}

		log.Printf("Notice - new UID was specified for the same property name '%s' - resetting value (recreating the property)", bindingProperty.Path())
		return property, nil
	}

	// we don't care about this error, either the property is found or we create it
	property, _ := modelEntity.FindPropertyByName(bindingProperty.Name)

	// handle uid request
	if bindingProperty.uidRequest {
		if property != nil {
			uid, err := property.Id.GetUid()
			if err != nil {
				return nil, err
			}
			newUid, err := modelInfo.GenerateUid()
			if err != nil {
				return nil, err
			}

			// handle "reset property data" use-case - adding a new UID to an existing property
			return nil, fmt.Errorf(`uid annotation value must not be empty on property %s, entity %s:
    [rename] apply the current UID %d
    [change/reset] apply a new UID %d`,
				bindingProperty.Name, bindingProperty.entity.Name, uid, newUid)
		}
		return nil, fmt.Errorf("uid annotation value must not be empty on an unknown property %s, entity %s",
			bindingProperty.Name, bindingProperty.entity.Name)
	}

	if property == nil {
		return modelEntity.CreateProperty()
	}

	return property, nil
}

func mergeModelProperty(bindingProperty *Property, modelProperty *modelinfo.Property) (err error) {
	modelProperty.Name = bindingProperty.Name

	// handle "reset property data" use-case - adding a new UID to an existing property
	if bindingProperty.Uid != 0 {
		id, _, err := modelProperty.Id.Get()
		if err != nil {
			return err
		}
		modelProperty.Id = modelinfo.CreateIdUid(id, bindingProperty.Uid)
	}

	if bindingProperty.Id, bindingProperty.Uid, err = modelProperty.Id.Get(); err != nil {
		return err
	}

	if bindingProperty.Index == nil {
		// if there shouldn't be an index
		if modelProperty.IndexId != nil {
			// if there originally was an index, remove it
			if err = modelProperty.RemoveIndex(); err != nil {
				return err
			}
		}
	} else {
		// if there should be an index, create it (or reuse an existing one)
		if modelProperty.IndexId == nil {
			if err = modelProperty.CreateIndex(); err != nil {
				return err
			}
		}

		if bindingProperty.Index.Id, bindingProperty.Index.Uid, err = modelProperty.IndexId.Get(); err != nil {
			return err
		}
	}

	if bindingProperty.Relation != nil {
		modelProperty.RelationTarget = bindingProperty.Relation.Target.Name
	} else {
		modelProperty.RelationTarget = ""
	}

	modelProperty.Type = bindingProperty.ObType
	modelProperty.Flags = bindingProperty.ObFlagsCombined()

	return nil
}

func bindingPropertyExists(modelProperty *modelinfo.Property, bindingEntity *Entity) bool {
	for _, bindingProperty := range bindingEntity.Properties {
		if bindingProperty.Name == modelProperty.Name {
			return true
		}
	}

	return false
}

func getModelRelation(bindingRelation *StandaloneRelation, modelEntity *modelinfo.Entity) (*modelinfo.StandaloneRelation, error) {
	if bindingRelation.Uid != 0 {
		return modelEntity.FindRelationByUid(bindingRelation.Uid)
	}

	// we don't care about this error, either the relation is found or we create it
	relation, _ := modelEntity.FindRelationByName(bindingRelation.Name)

	// handle uid request
	if bindingRelation.uidRequest {
		var errInfo string
		if relation != nil {
			uid, err := relation.Id.GetUid()
			if err != nil {
				return nil, err
			}
			errInfo = fmt.Sprintf("model relation UID = %d", uid)
		} else {
			errInfo = "relation not found in the model"
		}
		return nil, fmt.Errorf("uid annotation value must not be empty (%s) on relation %s, entity %s",
			errInfo, bindingRelation.Name, modelEntity.Name)
	}

	if relation == nil {
		return modelEntity.CreateRelation()
	}

	return relation, nil
}

func mergeModelRelation(bindingRelation *StandaloneRelation, modelRelation *modelinfo.StandaloneRelation, modelInfo *modelinfo.ModelInfo) (err error) {
	modelRelation.Name = bindingRelation.Name

	if bindingRelation.Id, bindingRelation.Uid, err = modelRelation.Id.Get(); err != nil {
		return err
	}

	// find the target entity & read it's ID/UID for the binding code
	if targetEntity, err := modelInfo.FindEntityByName(bindingRelation.Target.Name); err != nil {
		return err
	} else if bindingRelation.Target.Id, bindingRelation.Target.Uid, err = targetEntity.Id.Get(); err != nil {
		return err
	} else {
		modelRelation.SetTarget(targetEntity)
	}

	return nil
}

func bindingRelationExists(modelRelation *modelinfo.StandaloneRelation, bindingEntity *Entity) bool {
	for _, bindingRelation := range bindingEntity.Relations {
		if bindingRelation.Name == modelRelation.Name {
			return true
		}
	}

	return false
}
