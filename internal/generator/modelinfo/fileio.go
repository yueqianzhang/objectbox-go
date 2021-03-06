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

package modelinfo

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

// LoadOrCreateModel reads a model file or creates a new one if it doesn't exist
func LoadOrCreateModel(path string) (model *ModelInfo, err error) {
	if fileExists(path) {
		return loadModelFromJSONFile(path)
	}
	return createModelJSONFile(path)
}

// Close and unlock model
func (model *ModelInfo) Close() error {
	return model.file.Close()
}

// Write current model data to file
func (model *ModelInfo) Write() error {
	data, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return err
	}

	if err = model.file.Truncate(0); err != nil {
		return err
	}

	if _, err := model.file.WriteAt(data, 0); err != nil {
		return err
	}

	if err = model.file.Sync(); err != nil {
		return err
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func loadModelFromJSONFile(path string) (model *ModelInfo, err error) {
	model = &ModelInfo{}

	if model.file, err = os.OpenFile(path, os.O_RDWR, 0); err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(io.Reader(model.file))

	if err == nil {
		err = json.Unmarshal(data, model)
	}

	if err != nil {
		defer model.Close()
		return nil, fmt.Errorf("can't read file %s: %s", path, err)
	}

	// until objectbox-go 0.9 we didn't have model version in the file but it was basically version 4; recognize this
	if model.ModelVersion == 0 && model.MinimumParserVersion == 0 && len(model.Note1) == 0 {
		model.ModelVersion = 4
		model.MinimumParserVersion = 4
	}

	model.fillMissing()

	return model, nil
}

func createModelJSONFile(path string) (model *ModelInfo, err error) {
	model = createModelInfo()

	// create a file handle so to have an exclusive access
	if model.file, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600); err != nil {
		return nil, err
	}

	// write it with initial content (so that we know it's writable & it would have correct contents on next tool run)
	if err = model.Write(); err != nil {
		defer model.Close()
		return nil, fmt.Errorf("can't write file %s: %s", path, err)
	}

	return model, nil
}
