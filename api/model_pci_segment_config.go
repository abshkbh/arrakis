/*
Cloud Hypervisor API

Local HTTP based API for managing and inspecting a cloud-hypervisor virtual machine.

API version: 0.3.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"encoding/json"
	"bytes"
	"fmt"
)

// checks if the PciSegmentConfig type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &PciSegmentConfig{}

// PciSegmentConfig struct for PciSegmentConfig
type PciSegmentConfig struct {
	PciSegment int32 `json:"pci_segment"`
	Mmio32ApertureWeight *int32 `json:"mmio32_aperture_weight,omitempty"`
	Mmio64ApertureWeight *int32 `json:"mmio64_aperture_weight,omitempty"`
}

type _PciSegmentConfig PciSegmentConfig

// NewPciSegmentConfig instantiates a new PciSegmentConfig object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewPciSegmentConfig(pciSegment int32) *PciSegmentConfig {
	this := PciSegmentConfig{}
	this.PciSegment = pciSegment
	return &this
}

// NewPciSegmentConfigWithDefaults instantiates a new PciSegmentConfig object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewPciSegmentConfigWithDefaults() *PciSegmentConfig {
	this := PciSegmentConfig{}
	return &this
}

// GetPciSegment returns the PciSegment field value
func (o *PciSegmentConfig) GetPciSegment() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.PciSegment
}

// GetPciSegmentOk returns a tuple with the PciSegment field value
// and a boolean to check if the value has been set.
func (o *PciSegmentConfig) GetPciSegmentOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.PciSegment, true
}

// SetPciSegment sets field value
func (o *PciSegmentConfig) SetPciSegment(v int32) {
	o.PciSegment = v
}

// GetMmio32ApertureWeight returns the Mmio32ApertureWeight field value if set, zero value otherwise.
func (o *PciSegmentConfig) GetMmio32ApertureWeight() int32 {
	if o == nil || IsNil(o.Mmio32ApertureWeight) {
		var ret int32
		return ret
	}
	return *o.Mmio32ApertureWeight
}

// GetMmio32ApertureWeightOk returns a tuple with the Mmio32ApertureWeight field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PciSegmentConfig) GetMmio32ApertureWeightOk() (*int32, bool) {
	if o == nil || IsNil(o.Mmio32ApertureWeight) {
		return nil, false
	}
	return o.Mmio32ApertureWeight, true
}

// HasMmio32ApertureWeight returns a boolean if a field has been set.
func (o *PciSegmentConfig) HasMmio32ApertureWeight() bool {
	if o != nil && !IsNil(o.Mmio32ApertureWeight) {
		return true
	}

	return false
}

// SetMmio32ApertureWeight gets a reference to the given int32 and assigns it to the Mmio32ApertureWeight field.
func (o *PciSegmentConfig) SetMmio32ApertureWeight(v int32) {
	o.Mmio32ApertureWeight = &v
}

// GetMmio64ApertureWeight returns the Mmio64ApertureWeight field value if set, zero value otherwise.
func (o *PciSegmentConfig) GetMmio64ApertureWeight() int32 {
	if o == nil || IsNil(o.Mmio64ApertureWeight) {
		var ret int32
		return ret
	}
	return *o.Mmio64ApertureWeight
}

// GetMmio64ApertureWeightOk returns a tuple with the Mmio64ApertureWeight field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PciSegmentConfig) GetMmio64ApertureWeightOk() (*int32, bool) {
	if o == nil || IsNil(o.Mmio64ApertureWeight) {
		return nil, false
	}
	return o.Mmio64ApertureWeight, true
}

// HasMmio64ApertureWeight returns a boolean if a field has been set.
func (o *PciSegmentConfig) HasMmio64ApertureWeight() bool {
	if o != nil && !IsNil(o.Mmio64ApertureWeight) {
		return true
	}

	return false
}

// SetMmio64ApertureWeight gets a reference to the given int32 and assigns it to the Mmio64ApertureWeight field.
func (o *PciSegmentConfig) SetMmio64ApertureWeight(v int32) {
	o.Mmio64ApertureWeight = &v
}

func (o PciSegmentConfig) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o PciSegmentConfig) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["pci_segment"] = o.PciSegment
	if !IsNil(o.Mmio32ApertureWeight) {
		toSerialize["mmio32_aperture_weight"] = o.Mmio32ApertureWeight
	}
	if !IsNil(o.Mmio64ApertureWeight) {
		toSerialize["mmio64_aperture_weight"] = o.Mmio64ApertureWeight
	}
	return toSerialize, nil
}

func (o *PciSegmentConfig) UnmarshalJSON(data []byte) (err error) {
	// This validates that all required properties are included in the JSON object
	// by unmarshalling the object into a generic map with string keys and checking
	// that every required field exists as a key in the generic map.
	requiredProperties := []string{
		"pci_segment",
	}

	allProperties := make(map[string]interface{})

	err = json.Unmarshal(data, &allProperties)

	if err != nil {
		return err;
	}

	for _, requiredProperty := range(requiredProperties) {
		if _, exists := allProperties[requiredProperty]; !exists {
			return fmt.Errorf("no value given for required property %v", requiredProperty)
		}
	}

	varPciSegmentConfig := _PciSegmentConfig{}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&varPciSegmentConfig)

	if err != nil {
		return err
	}

	*o = PciSegmentConfig(varPciSegmentConfig)

	return err
}

type NullablePciSegmentConfig struct {
	value *PciSegmentConfig
	isSet bool
}

func (v NullablePciSegmentConfig) Get() *PciSegmentConfig {
	return v.value
}

func (v *NullablePciSegmentConfig) Set(val *PciSegmentConfig) {
	v.value = val
	v.isSet = true
}

func (v NullablePciSegmentConfig) IsSet() bool {
	return v.isSet
}

func (v *NullablePciSegmentConfig) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullablePciSegmentConfig(val *PciSegmentConfig) *NullablePciSegmentConfig {
	return &NullablePciSegmentConfig{value: val, isSet: true}
}

func (v NullablePciSegmentConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullablePciSegmentConfig) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


