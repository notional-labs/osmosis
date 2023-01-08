package concentrated_liquidity

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmoutils"
	"github.com/osmosis-labs/osmosis/v13/x/concentrated-liquidity/internal/math"
	"github.com/osmosis-labs/osmosis/v13/x/concentrated-liquidity/model"
	types "github.com/osmosis-labs/osmosis/v13/x/concentrated-liquidity/types"
)

// initOrUpdateTick retrieves the tickInfo from the specified tickIndex and updates both the liquidityNet and LiquidityGross.
// if we are initializing or updating an upper tick, we subtract the liquidityIn from the LiquidityNet
// if we are initializing or updating an lower tick, we add the liquidityIn from the LiquidityNet
func (k Keeper) initOrUpdateTick(ctx sdk.Context, poolId uint64, tickIndex int64, liquidityIn sdk.Dec, upper bool, incentiveIDsCommittedTo []uint64) (err error) {
	if !k.poolExists(ctx, poolId) {
		return types.PoolNotFoundError{PoolId: poolId}
	}

	tickInfo, err := k.getTickInfo(ctx, poolId, tickIndex)
	if err != nil {
		return err
	}

	pool, err := k.getPoolById(ctx, poolId)
	if err != nil {
		return err
	}

	// if the following is true, we are either initializing a tick for the first time or initializing it after it was inactive.
	// therefore, we must set the seconds inactive to the length of time the pool has existed.
	if tickInfo.LiquidityGross.Equal(sdk.ZeroDec()) && tickInfo.LiquidityNet.Equal(sdk.ZeroDec()) {
		// pool, err := k.getPoolById(ctx, poolId)
		// if err != nil {
		// 	return err
		// }

		tickInfo.SecondsInactive = ctx.BlockTime().Sub(pool.GetTimeOfCreation())
	}

	// calculate liquidityGross, which does not care about whether liquidityIn is positive or negative
	liquidityBefore := tickInfo.LiquidityGross
	fmt.Printf("liquidityBefore before: %v \n", liquidityBefore)

	// note that liquidityIn can be either positive or negative.
	// If negative, this would work as a subtraction from liquidityBefore
	liquidityAfter := math.AddLiquidity(liquidityBefore, liquidityIn)

	tickInfo.LiquidityGross = liquidityAfter

	// calculate liquidityNet, which we take into account and track depending on whether liquidityIn is positive or negative
	if upper {
		tickInfo.LiquidityNet = tickInfo.LiquidityNet.Sub(liquidityIn)
	} else {
		tickInfo.LiquidityNet = tickInfo.LiquidityNet.Add(liquidityIn)
	}

	// Incentivized Liquidity

	if len(tickInfo.TickIncentivizedLiquidityRecords) == 0 {
		// If the tickInfo object has no incentivized liquidity records, create new records for
		// each of the pool's incentivized liquidity records and set the gross and net liquidity
		// to zero.
		poolIncentivizedLiquidityRecord := pool.GetPoolIncentivizedLiquidityRecords()
		for _, record := range poolIncentivizedLiquidityRecord {
			tickInfo.TickIncentivizedLiquidityRecords = append(tickInfo.TickIncentivizedLiquidityRecords, model.TickIncentivizedLiquidityRecord{
				ID:                         record.ID,
				IncentivizedLiquidityGross: sdk.ZeroDec(),
				IncentivizedLiquidityNet:   sdk.ZeroDec(),
			})
		}
	} else if len(pool.GetPoolIncentivizedLiquidityRecords()) != len(tickInfo.TickIncentivizedLiquidityRecords) {
		// If the tickInfo object has a different number of incentivized liquidity records than
		// the pool, create new records for any missing records in the tickInfo object and set
		// the gross and net liquidity to zero.
		poolIncentivizedLiquidityRecord := pool.GetPoolIncentivizedLiquidityRecords()
		var newTickIncentivizedLiquidityRecords []model.TickIncentivizedLiquidityRecord
		for i, record := range poolIncentivizedLiquidityRecord {
			if tickInfo.TickIncentivizedLiquidityRecords[i].ID == record.ID {
				newTickIncentivizedLiquidityRecords = append(newTickIncentivizedLiquidityRecords, tickInfo.TickIncentivizedLiquidityRecords[i])
			} else {
				newTickIncentivizedLiquidityRecords = append(tickInfo.TickIncentivizedLiquidityRecords, model.TickIncentivizedLiquidityRecord{
					ID:                         record.ID,
					IncentivizedLiquidityGross: sdk.ZeroDec(),
					IncentivizedLiquidityNet:   sdk.ZeroDec(),
				})
			}
		}
	}
	// Otherwise, update the incentivized liquidity records in the tickInfo object based on
	// the given `incentiveIDsCommittedTo` and the amount of liquidity being added.
	for i, incentiveID := range incentiveIDsCommittedTo {
		if tickInfo.TickIncentivizedLiquidityRecords[i].ID == incentiveID {
			incentivizedLiquidityBefore := tickInfo.TickIncentivizedLiquidityRecords[i].IncentivizedLiquidityGross
			incentivizedLiquidityAfter := math.AddLiquidity(incentivizedLiquidityBefore, liquidityIn)
			tickInfo.TickIncentivizedLiquidityRecords[i].IncentivizedLiquidityGross = incentivizedLiquidityAfter
			if upper {
				tickInfo.TickIncentivizedLiquidityRecords[i].IncentivizedLiquidityNet = tickInfo.TickIncentivizedLiquidityRecords[i].IncentivizedLiquidityNet.Sub(liquidityIn)
			} else {
				tickInfo.TickIncentivizedLiquidityRecords[i].IncentivizedLiquidityNet = tickInfo.TickIncentivizedLiquidityRecords[i].IncentivizedLiquidityNet.Add(liquidityIn)
			}
		}
	}

	k.SetTickInfo(ctx, poolId, tickIndex, tickInfo)

	return nil
}

func (k Keeper) crossTick(ctx sdk.Context, poolId uint64, tickIndex int64) (liquidityDelta sdk.Dec, err error) {
	tickInfo, err := k.getTickInfo(ctx, poolId, tickIndex)
	if err != nil {
		return sdk.Dec{}, err
	}

	pool, err := k.getPoolById(ctx, poolId)
	if err != nil {
		return sdk.Dec{}, err
	}

	//newSecondsInactive := ctx.BlockTime().Sub(pool.GetTimeOfCreation()) - tickInfo.SecondsInactive
	newSecondsInactive := ctx.BlockTime().Sub(pool.GetTimeOfCreation())
	tickInfo.SecondsInactive = newSecondsInactive

	// Update seconds per liquidity outside
	// fmt.Printf("Seconds inactive: %v \n", tickInfo.SecondsInactive.Seconds())
	// fmt.Printf("Liquidity gross: %v \n", tickInfo.LiquidityGross)
	for _, tickIncentivizedLiquidityRecord := range tickInfo.TickIncentivizedLiquidityRecords {
		tickIncentivizedLiquidityRecord.SecondsPerIncentivizedLiquidityOutside = sdk.MustNewDecFromStr(fmt.Sprintf("%f", tickInfo.SecondsInactive.Seconds())).Quo(tickIncentivizedLiquidityRecord.IncentivizedLiquidityGross)
	}
	k.SetTickInfo(ctx, poolId, tickIndex, tickInfo)

	// Set new global seconds per liquidity
	poolIncentivizedLiquidityRecords := pool.GetPoolIncentivizedLiquidityRecords()
	for i, poolRecord := range poolIncentivizedLiquidityRecords {
		poolRecord.SecondsPerIncentivizedLiquidityGlobal = poolRecord.SecondsPerIncentivizedLiquidityGlobal.Add(tickInfo.TickIncentivizedLiquidityRecords[i].SecondsPerIncentivizedLiquidityOutside)
	}
	pool.SetPoolIncentivizedLiquidityRecords(poolIncentivizedLiquidityRecords)
	err = k.setPool(ctx, pool)
	if err != nil {
		return sdk.Dec{}, err
	}

	return tickInfo.LiquidityNet, nil
}

// getTickInfo gets tickInfo given poolId and tickIndex. Returns a boolean field that returns true if value is found for given key.
func (k Keeper) getTickInfo(ctx sdk.Context, poolId uint64, tickIndex int64) (tickInfo model.TickInfo, err error) {
	store := ctx.KVStore(k.storeKey)
	tickStruct := model.TickInfo{}
	key := types.KeyTick(poolId, tickIndex)
	if !k.poolExists(ctx, poolId) {
		return model.TickInfo{}, types.PoolNotFoundError{PoolId: poolId}
	}

	found, err := osmoutils.Get(store, key, &tickStruct)
	// return 0 values if key has not been initialized
	if !found {
		return model.TickInfo{LiquidityGross: sdk.ZeroDec(), LiquidityNet: sdk.ZeroDec()}, err
	}
	if err != nil {
		return tickStruct, err
	}

	return tickStruct, nil
}

func (k Keeper) SetTickInfo(ctx sdk.Context, poolId uint64, tickIndex int64, tickInfo model.TickInfo) {
	store := ctx.KVStore(k.storeKey)
	key := types.KeyTick(poolId, tickIndex)
	osmoutils.MustSet(store, key, &tickInfo)
}

// validateTickInRangeIsValid validates that given ticks are valid.
// That is, both lower and upper ticks are within types.MinTick and types.MaxTick.
// Also, lower tick must be less than upper tick.
// Returns error if validation fails. Otherwise, nil.
// TODO: test
func validateTickRangeIsValid(tickSpacing uint64, lowerTick int64, upperTick int64) error {
	// Check if the lower and upper tick values are divisible by the tick spacing.
	if lowerTick%int64(tickSpacing) != 0 || upperTick%int64(tickSpacing) != 0 {
		return types.TickSpacingError{LowerTick: lowerTick, UpperTick: upperTick, TickSpacing: tickSpacing}
	}

	// Check if the lower tick value is within the valid range of MinTick to MaxTick.
	if lowerTick < types.MinTick || lowerTick >= types.MaxTick {
		return types.InvalidTickError{Tick: lowerTick, IsLower: true}
	}

	// Check if the upper tick value is within the valid range of MinTick to MaxTick.
	if upperTick > types.MaxTick || upperTick <= types.MinTick {
		return types.InvalidTickError{Tick: upperTick, IsLower: false}
	}

	// Check if the lower tick value is greater than or equal to the upper tick value.
	if lowerTick >= upperTick {
		return types.InvalidLowerUpperTickError{LowerTick: lowerTick, UpperTick: upperTick}
	}
	return nil
}
