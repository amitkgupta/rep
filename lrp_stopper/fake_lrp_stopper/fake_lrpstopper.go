// This file was generated by counterfeiter
package fake_lrp_stopper

import (
	"sync"
	. "github.com/cloudfoundry-incubator/rep/lrp_stopper"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
)

type FakeLRPStopper struct {
	StopInstanceStub        func(stopInstance models.StopLRPInstance) error
	stopInstanceMutex       sync.RWMutex
	stopInstanceArgsForCall []struct {
		arg1 models.StopLRPInstance
	}
	stopInstanceReturns struct {
		result1 error
	}
}

func (fake *FakeLRPStopper) StopInstance(arg1 models.StopLRPInstance) error {
	fake.stopInstanceMutex.Lock()
	defer fake.stopInstanceMutex.Unlock()
	fake.stopInstanceArgsForCall = append(fake.stopInstanceArgsForCall, struct {
		arg1 models.StopLRPInstance
	}{arg1})
	if fake.StopInstanceStub != nil {
		return fake.StopInstanceStub(arg1)
	} else {
		return fake.stopInstanceReturns.result1
	}
}

func (fake *FakeLRPStopper) StopInstanceCallCount() int {
	fake.stopInstanceMutex.RLock()
	defer fake.stopInstanceMutex.RUnlock()
	return len(fake.stopInstanceArgsForCall)
}

func (fake *FakeLRPStopper) StopInstanceArgsForCall(i int) models.StopLRPInstance {
	fake.stopInstanceMutex.RLock()
	defer fake.stopInstanceMutex.RUnlock()
	return fake.stopInstanceArgsForCall[i].arg1
}

func (fake *FakeLRPStopper) StopInstanceReturns(result1 error) {
	fake.stopInstanceReturns = struct {
		result1 error
	}{result1}
}

var _ LRPStopper = new(FakeLRPStopper)
