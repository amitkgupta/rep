package auctionrep

import (
	"errors"
	"math"
	"sync"

	"github.com/cloudfoundry-incubator/auction/auctiontypes"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
)

type AuctionRep struct {
	repGuid  string
	delegate auctiontypes.AuctionRepDelegate
	lock     *sync.Mutex
}

type InstanceScoreInfo struct {
	RemainingResources                auctiontypes.Resources
	TotalResources                    auctiontypes.Resources
	NumInstancesOnRepForProcessGuid   int
	NumInstancesDesiredForProcessGuid int
	Index                             int
	ProcessGuid                       string
	RepAZNumber                       int
	TotalNumAZs                       int
}

func New(repGuid string, delegate auctiontypes.AuctionRepDelegate) *AuctionRep {
	return &AuctionRep{
		repGuid:  repGuid,
		delegate: delegate,
		lock:     &sync.Mutex{},
	}
}

func (rep *AuctionRep) Guid() string {
	return rep.repGuid
}

func (rep *AuctionRep) AZNumber() int {
	return rep.delegate.AZNumber()
}

// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) BidForStartAuction(startAuctionInfo auctiontypes.StartAuctionInfo) (float64, error) {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	instanceScoreInfo, err := rep.instanceScoreInfo(
		startAuctionInfo.ProcessGuid,
		startAuctionInfo.NumInstances,
		startAuctionInfo.Index,
		startAuctionInfo.NumAZs,
	)
	if err != nil {
		return 0, err
	}

	err = rep.satisfiesConstraints(startAuctionInfo, instanceScoreInfo)
	if err != nil {
		return 0, err
	}

	return rep.startAuctionBid(instanceScoreInfo), nil
}

// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) RebidThenTentativelyReserve(startAuctionInfo auctiontypes.StartAuctionInfo) (float64, error) {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	instanceScoreInfo, err := rep.instanceScoreInfo(
		startAuctionInfo.ProcessGuid,
		startAuctionInfo.NumInstances,
		startAuctionInfo.Index,
		startAuctionInfo.NumAZs,
	)
	if err != nil {
		return 0, err
	}

	err = rep.satisfiesConstraints(startAuctionInfo, instanceScoreInfo)
	if err != nil {
		return 0, err
	}

	bid := rep.startAuctionBid(instanceScoreInfo)

	//then reserve
	err = rep.delegate.Reserve(startAuctionInfo)
	if err != nil {
		return 0, err
	}

	return bid, nil
}

// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) ReleaseReservation(startAuctionInfo auctiontypes.StartAuctionInfo) error {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	return rep.delegate.ReleaseReservation(startAuctionInfo)
}

// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) Run(startAuction models.LRPStartAuction) error {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	return rep.delegate.Run(startAuction)
}

// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) BidForStopAuction(stopAuctionInfo auctiontypes.StopAuctionInfo) (float64, []string, error) {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	instanceScoreInfo, err := rep.instanceScoreInfo(
		stopAuctionInfo.ProcessGuid,
		stopAuctionInfo.NumInstances,
		stopAuctionInfo.Index,
		stopAuctionInfo.NumAZs,
	)
	if err != nil {
		return 0, nil, err
	}

	instanceGuids, err := rep.delegate.InstanceGuidsForProcessGuidAndIndex(stopAuctionInfo.ProcessGuid, stopAuctionInfo.Index)
	if err != nil {
		return 0, nil, err
	}

	err = rep.isRunningProcessIndex(instanceGuids)
	if err != nil {
		return 0, nil, err
	}

	return rep.stopAuctionBid(instanceScoreInfo), instanceGuids, nil
}

// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) Stop(stopInstance models.StopLRPInstance) error {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	return rep.delegate.Stop(stopInstance)
}

// simulation-only
func (rep *AuctionRep) TotalResources() auctiontypes.Resources {
	totalResources, _ := rep.delegate.TotalResources()
	return totalResources
}

// simulation-only
// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) Reset() {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	simDelegate, ok := rep.delegate.(auctiontypes.SimulationAuctionRepDelegate)
	if !ok {
		println("not reseting")
		return
	}
	simDelegate.SetSimulatedInstances([]auctiontypes.SimulatedInstance{})
}

// simulation-only
// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) SetSimulatedInstances(instances []auctiontypes.SimulatedInstance) {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	simDelegate, ok := rep.delegate.(auctiontypes.SimulationAuctionRepDelegate)
	if !ok {
		println("not setting instances")
		return
	}
	simDelegate.SetSimulatedInstances(instances)
}

// simulation-only
// must lock here; the publicly visible operations should be atomic
func (rep *AuctionRep) SimulatedInstances() []auctiontypes.SimulatedInstance {
	rep.lock.Lock()
	defer rep.lock.Unlock()

	simDelegate, ok := rep.delegate.(auctiontypes.SimulationAuctionRepDelegate)
	if !ok {
		println("not fetching instances")
		return []auctiontypes.SimulatedInstance{}
	}
	return simDelegate.SimulatedInstances()
}

// private internals -- no locks here
func (rep *AuctionRep) instanceScoreInfo(processGuid string, numInstances int, index int, totalNumAZs int) (InstanceScoreInfo, error) {
	remaining, err := rep.delegate.RemainingResources()
	if err != nil {
		return InstanceScoreInfo{}, err
	}

	total, err := rep.delegate.TotalResources()
	if err != nil {
		return InstanceScoreInfo{}, err
	}

	nInstancesOnRep, err := rep.delegate.NumInstancesForProcessGuid(processGuid)
	if err != nil {
		return InstanceScoreInfo{}, err
	}

	repAZNumber := rep.delegate.AZNumber()

	return InstanceScoreInfo{
		RemainingResources:                remaining,
		TotalResources:                    total,
		NumInstancesOnRepForProcessGuid:   nInstancesOnRep,
		NumInstancesDesiredForProcessGuid: numInstances,
		Index:       index,
		ProcessGuid: processGuid,
		RepAZNumber: repAZNumber,
		TotalNumAZs: totalNumAZs,
	}, nil
}

// private internals -- no locks here
func (rep *AuctionRep) satisfiesConstraints(startAuctionInfo auctiontypes.StartAuctionInfo, instanceScoreInfo InstanceScoreInfo) error {
	remaining := instanceScoreInfo.RemainingResources
	hasEnoughMemory := remaining.MemoryMB >= startAuctionInfo.MemoryMB
	hasEnoughDisk := remaining.DiskMB >= startAuctionInfo.DiskMB
	hasEnoughContainers := remaining.Containers > 0

	if hasEnoughMemory && hasEnoughDisk && hasEnoughContainers {
		return nil
	} else {
		return auctiontypes.InsufficientResources
	}
}

// private internals -- no locks here
func (rep *AuctionRep) isRunningProcessIndex(instanceGuids []string) error {
	if len(instanceGuids) == 0 {
		return errors.New("not-running-instance")
	}
	return nil
}

// private internals -- no locks here
func (rep *AuctionRep) startAuctionBid(instanceScoreInfo InstanceScoreInfo) float64 {
	remaining := instanceScoreInfo.RemainingResources
	total := instanceScoreInfo.TotalResources

	fractionUsedContainers := 1.0 - float64(remaining.Containers)/float64(total.Containers)
	fractionUsedDisk := 1.0 - float64(remaining.DiskMB)/float64(total.DiskMB)
	fractionUsedMemory := 1.0 - float64(remaining.MemoryMB)/float64(total.MemoryMB)
	fractionInstancesForProcessGuid := float64(instanceScoreInfo.NumInstancesOnRepForProcessGuid) / float64(instanceScoreInfo.NumInstancesDesiredForProcessGuid)
	azBalancingTerm := -math.Mod(
		float64(instanceScoreInfo.Index+hash(instanceScoreInfo.ProcessGuid)+instanceScoreInfo.RepAZNumber),
		float64(instanceScoreInfo.TotalNumAZs),
	)

	return (1.0/3.0)*fractionUsedContainers +
		(1.0/3.0)*fractionUsedDisk +
		(1.0/3.0)*fractionUsedMemory +
		(1.0/1.0)*fractionInstancesForProcessGuid +
		(1.0/1.0)*azBalancingTerm
}

func hash(s string) int {
	h := 0
	for _, b := range []byte(s) {
		h += int(b)
	}
	return h
}

// private internals -- no locks here
func (rep *AuctionRep) stopAuctionBid(instanceScoreInfo InstanceScoreInfo) float64 {
	remaining := instanceScoreInfo.RemainingResources
	total := instanceScoreInfo.TotalResources

	fractionUsedContainers := 1.0 - float64(remaining.Containers)/float64(total.Containers)
	fractionUsedDisk := 1.0 - float64(remaining.DiskMB)/float64(total.DiskMB)
	fractionUsedMemory := 1.0 - float64(remaining.MemoryMB)/float64(total.MemoryMB)

	return (1.0/3.0)*fractionUsedContainers +
		(1.0/3.0)*fractionUsedDisk +
		(1.0/3.0)*fractionUsedMemory +
		(1.0/1.0)*float64(instanceScoreInfo.NumInstancesOnRepForProcessGuid)
}
