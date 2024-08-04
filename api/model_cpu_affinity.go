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

// checks if the CpuAffinity type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &CpuAffinity{}

// CpuAffinity struct for CpuAffinity
type CpuAffinity struct {
	Vcpu int32 `json:"vcpu"`
	HostCpus []int32 `json:"host_cpus"`
}

type _CpuAffinity CpuAffinity

// NewCpuAffinity instantiates a new CpuAffinity object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewCpuAffinity(vcpu int32, hostCpus []int32) *CpuAffinity {
	this := CpuAffinity{}
	this.Vcpu = vcpu
	this.HostCpus = hostCpus
	return &this
}

// NewCpuAffinityWithDefaults instantiates a new CpuAffinity object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewCpuAffinityWithDefaults() *CpuAffinity {
	this := CpuAffinity{}
	return &this
}

// GetVcpu returns the Vcpu field value
func (o *CpuAffinity) GetVcpu() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.Vcpu
}

// GetVcpuOk returns a tuple with the Vcpu field value
// and a boolean to check if the value has been set.
func (o *CpuAffinity) GetVcpuOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Vcpu, true
}

// SetVcpu sets field value
func (o *CpuAffinity) SetVcpu(v int32) {
	o.Vcpu = v
}

// GetHostCpus returns the HostCpus field value
func (o *CpuAffinity) GetHostCpus() []int32 {
	if o == nil {
		var ret []int32
		return ret
	}

	return o.HostCpus
}

// GetHostCpusOk returns a tuple with the HostCpus field value
// and a boolean to check if the value has been set.
func (o *CpuAffinity) GetHostCpusOk() ([]int32, bool) {
	if o == nil {
		return nil, false
	}
	return o.HostCpus, true
}

// SetHostCpus sets field value
func (o *CpuAffinity) SetHostCpus(v []int32) {
	o.HostCpus = v
}

func (o CpuAffinity) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o CpuAffinity) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["vcpu"] = o.Vcpu
	toSerialize["host_cpus"] = o.HostCpus
	return toSerialize, nil
}

func (o *CpuAffinity) UnmarshalJSON(data []byte) (err error) {
	// This validates that all required properties are included in the JSON object
	// by unmarshalling the object into a generic map with string keys and checking
	// that every required field exists as a key in the generic map.
	requiredProperties := []string{
		"vcpu",
		"host_cpus",
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

	varCpuAffinity := _CpuAffinity{}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&varCpuAffinity)

	if err != nil {
		return err
	}

	*o = CpuAffinity(varCpuAffinity)

	return err
}

type NullableCpuAffinity struct {
	value *CpuAffinity
	isSet bool
}

func (v NullableCpuAffinity) Get() *CpuAffinity {
	return v.value
}

func (v *NullableCpuAffinity) Set(val *CpuAffinity) {
	v.value = val
	v.isSet = true
}

func (v NullableCpuAffinity) IsSet() bool {
	return v.isSet
}

func (v *NullableCpuAffinity) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableCpuAffinity(val *CpuAffinity) *NullableCpuAffinity {
	return &NullableCpuAffinity{value: val, isSet: true}
}

func (v NullableCpuAffinity) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableCpuAffinity) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}


