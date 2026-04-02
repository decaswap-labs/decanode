//go:build chainnet
// +build chainnet

package constants

func init() {
	int64Overrides = map[ConstantName]int64{
		ChurnInterval:              50400,
		OperationalVotesMin:        1,
		MinDecaPoolDepth:           1_00000000,
		MinimumBondInDeca:          200_000_00000000,
		PoolCycle:                  720,
		NumberOfNewNodesPerChurn:   4,
		MintSynths:                 1,
		BurnSynths:                 1,
		MaxDecaSupply:              500_000_000_00000000,
		MultipleAffiliatesMaxCount: 5,
	}
	stringOverrides = map[ConstantName]string{
		DevFundAddress:      "cthor1jydyvc8wt84jcdl6qex0ps9czfmcaqurljevkt",
		OverSolvencyAddress: "cthor1jydyvc8wt84jcdl6qex0ps9czfmcaqurljevkt",
	}
}
