package gateway

var (
	btState *BeaconTowerState
)

type BeaconTowerState struct {
	BucketId int64
}

func buildState() {
	btState = new(BeaconTowerState)
}
