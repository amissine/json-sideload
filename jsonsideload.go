package jsonsideload

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Unmarshal - maps sideloaded JSON to the given model
func Unmarshal(jsonPayload []byte, model interface{}) error {
	var sourceMap map[string]interface{}
	err := json.Unmarshal((jsonPayload), &sourceMap)
	if err != nil {
		return errors.New("Malformed JSON provided")
	}
	return unMarshalNode(sourceMap, sourceMap, reflect.ValueOf(model))
}

const (
	annotationJSONSideload    = "jsonsideload"
	annotationInclude         = "include"
	annotationIncludes        = "includes"
	annotationHasOneRelation  = "hasone"
	annotationHasManyRelation = "hasmany"
)

func unMarshalNode(sourceMap, mapToParse map[string]interface{}, model reflect.Value) (err error) {
	// recovering for any wrong representation in struct
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Data is not a jsonsideload representation of '%v'", model.Type())
		}
	}()

	// First, doing a json unmarshal to make sure all primitive types are mapped correct
	jsonString, err := json.Marshal(mapToParse)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonString, model.Interface())
	if err != nil {
		return err
	}
	modelValue := model.Elem()
	modelType := model.Type().Elem()

	var er error
	// Now going through all the fields of the struct
	for i := 0; i < modelValue.NumField(); i++ {
		fieldType := modelType.Field(i)
		tag := fieldType.Tag.Get(annotationJSONSideload)
		if tag == "" { // Ignoring the fields which doesn't have 'jsonsideload' tags
			continue
		}

		fieldValue := modelValue.Field(i)
		args := strings.Split(tag, ",")
		if len(args) < 1 { // Error, if there aren't any realationship with the tag
			er = errors.New("Bad jsonsideload struct tag format")
			break
		}
		annotation := args[0]

		// annotation includes means the object is already nested and not sideloaded
		if annotation == annotationInclude {
			if fieldValue.Kind() != reflect.Ptr { // Only pointer types are allowed in struct
				return fmt.Errorf("Expecting pointer type for %s in struct", fieldType.Name)
			}
			if len(args) < 2 {
				return fmt.Errorf("No relationship found in annotation for %s", fieldType.Name)
			}
			relation := args[1]
			var relationMap map[string]interface{}
			relationObj := mapToParse[relation]
			if relationObj != nil {
				if mapObj, ok := relationObj.(map[string]interface{}); ok {
					relationMap = mapObj
				}
			}
			m := reflect.New(fieldValue.Type().Elem())
			if relationMap != nil {
				if err := unMarshalNode(sourceMap, relationMap, m); err != nil {
					er = err
					break
				}
			}
			fieldValue.Set(m)
		} else if annotation == annotationIncludes { // annotation includes mean, the array is already nested and not sideloaded
			if len(args) < 2 {
				return fmt.Errorf("No relationship found in annotation for %s", fieldType.Name)
			}
			if fieldValue.Type().Elem().Kind() != reflect.Ptr {
				return fmt.Errorf("Expecting array of pointers for %s in struct", fieldType.Name)
			}
			relation := args[1]
			models := reflect.New(fieldValue.Type()).Elem()
			hasManyRelations := mapToParse[relation]
			if hasManyRelations != nil {
				if relationsArray, ok := hasManyRelations.([]interface{}); ok {
					for _, n := range relationsArray {
						m := reflect.New(fieldValue.Type().Elem().Elem())
						if err := unMarshalNode(sourceMap, n.(map[string]interface{}), m); err != nil {
							er = err
							break
						}
						models = reflect.Append(models, m)
					}
				}
			}
			fieldValue.Set(models)
		} else if annotation == annotationHasOneRelation { // hasone means, the relationship is sideloaded
			if fieldValue.Kind() != reflect.Ptr {
				return fmt.Errorf("Expecting pointer type for %s in struct", fieldType.Name)
			}
			if len(args) < 2 {
				return fmt.Errorf("No relationship found in annotation for %s", fieldType.Name)
			}
			var relationMap map[string]interface{}
			relation := args[1]
			relationID := mapToParse[args[2]]
			if relationID != nil { // using the relationID, search the source tree for the relationship
				valueMap := getValueFromSourceJSON(sourceMap, relation, relationID.(float64))
				if valueMap != nil {
					relationMap = valueMap.(map[string]interface{})
				}
			}
			m := reflect.New(fieldValue.Type().Elem())
			if relationMap != nil {
				if err := unMarshalNode(sourceMap, relationMap, m); err != nil {
					er = err
					break
				}
			}
			fieldValue.Set(m)
		} else if annotation == annotationHasManyRelation { // hasmany means, the relationships is sideloaded
			if len(args) < 2 {
				return fmt.Errorf("No relationship found in annotation for %s", fieldType.Name)
			}
			if fieldValue.Type().Elem().Kind() != reflect.Ptr {
				return fmt.Errorf("Expecting array of pointers for %s in struct", fieldType.Name)
			}
			models := reflect.New(fieldValue.Type()).Elem()
			relation := args[1]
			hasManyRelations := mapToParse[args[2]]
			if hasManyRelations != nil {
				if relationsArray, ok := hasManyRelations.([]interface{}); ok {
					for _, n := range relationsArray { // range on the array of relationship IDS and get each relationship from the source tree
						m := reflect.New(fieldValue.Type().Elem().Elem())
						relationMap := getValueFromSourceJSON(sourceMap, relation, n.(float64))
						if relationMap != nil {
							if err := unMarshalNode(sourceMap, relationMap.(map[string]interface{}), m); err != nil {
								er = err
								break
							}
							models = reflect.Append(models, m)
						}
					}
				}
			}
			fieldValue.Set(models)
		}
	}
	return er
}

// assign will take the value specified and assign it to the field; if
// field is expecting a ptr assign will assign a ptr.
func assign(field, value reflect.Value) {
	if field.Kind() == reflect.Ptr {
		field.Set(value)
	} else {
		field.Set(reflect.Indirect(value))
	}
}

// getValueFromSourceJSON - get the sideloaded value from the sourceJSON
func getValueFromSourceJSON(sourceJSON map[string]interface{}, key string, id float64) interface{} {
	valFromSourceJSON := sourceJSON[key]
	if valFromSourceJSON != nil {
		if valueArray, ok := sourceJSON[key].([]interface{}); ok {
			for _, v := range valueArray {
				if valueMap, ok := v.(map[string]interface{}); ok && valueMap["id"] == id {
					return v
				}
			}
		}
	}
	return nil
}
