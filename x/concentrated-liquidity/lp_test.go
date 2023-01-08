package concentrated_liquidity_test

import (
	"errors"
	"fmt"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	clmodel "github.com/osmosis-labs/osmosis/v13/x/concentrated-liquidity/model"
	types "github.com/osmosis-labs/osmosis/v13/x/concentrated-liquidity/types"
)

type lpTest struct {
	poolId                  uint64
	owner                   sdk.AccAddress
	currentTick             sdk.Int
	lowerTick               int64
	upperTick               int64
	currentSqrtP            sdk.Dec
	amount0Desired          sdk.Int
	amount0Minimum          sdk.Int
	amount0Expected         sdk.Int
	amount1Desired          sdk.Int
	amount1Minimum          sdk.Int
	amount1Expected         sdk.Int
	liquidityAmount         sdk.Dec
	tickSpacing             uint64
	incentiveIDsCommittedTo []uint64
	isNotFirstPosition      bool
	expectedError           error
}

var (
	baseCase = &lpTest{
		isNotFirstPosition:      false,
		poolId:                  1,
		currentTick:             DefaultCurrTick,
		lowerTick:               DefaultLowerTick,
		upperTick:               DefaultUpperTick,
		currentSqrtP:            DefaultCurrSqrtPrice,
		amount0Desired:          DefaultAmt0,
		amount0Minimum:          sdk.ZeroInt(),
		amount0Expected:         DefaultAmt0Expected,
		amount1Desired:          DefaultAmt1,
		amount1Minimum:          sdk.ZeroInt(),
		amount1Expected:         DefaultAmt1Expected,
		liquidityAmount:         DefaultLiquidityAmt,
		tickSpacing:             DefaultTickSpacing,
		incentiveIDsCommittedTo: DefaultIncentiveIDsCommittedTo,
	}
)

func (s *KeeperTestSuite) TestCreatePosition() {
	tests := map[string]lpTest{
		"base case": {},
		"base case without incentives": {
			incentiveIDsCommittedTo: []uint64{},
		},
		"create a position with non default tick spacing (10) with ticks that fall into tick spacing requirements": {
			lowerTick:       int64(84220),
			upperTick:       int64(86130),
			amount0Expected: sdk.NewInt(997568),
			liquidityAmount: sdk.MustNewDecFromStr("1514719247.706887470270366521"),
			tickSpacing:     10,
		},
		"error: non-existent pool": {
			poolId:        2,
			expectedError: types.PoolNotFoundError{PoolId: 2},
		},
		"error: lower tick out of bounds": {
			lowerTick:     types.MinTick - 1,
			expectedError: types.InvalidTickError{Tick: types.MinTick - 1, IsLower: true},
		},
		"error: upper tick out of bounds": {
			upperTick:     types.MaxTick + 1,
			expectedError: types.InvalidTickError{Tick: types.MaxTick + 1, IsLower: false},
		},
		"error: upper tick is below the lower tick, but both are in bounds": {
			lowerTick:     50,
			upperTick:     40,
			expectedError: types.InvalidLowerUpperTickError{LowerTick: 50, UpperTick: 40},
		},
		"error: amount of token 0 is smaller than minimum; should fail and not update state": {
			amount0Minimum: baseCase.amount0Expected.Mul(sdk.NewInt(2)),
			expectedError:  types.InsufficientLiquidityCreatedError{Actual: baseCase.amount0Expected, Minimum: baseCase.amount0Expected.Mul(sdk.NewInt(2)), IsTokenZero: true},
		},
		"error: amount of token 1 is smaller than minimum; should fail and not update state": {
			amount1Minimum: baseCase.amount1Expected.Mul(sdk.NewInt(2)),
			expectedError:  types.InsufficientLiquidityCreatedError{Actual: baseCase.amount1Expected, Minimum: baseCase.amount1Expected.Mul(sdk.NewInt(2))},
		},
		"error: a non first position with zero amount desired for both denoms should fail liquidity delta check": {
			isNotFirstPosition: true,
			amount0Desired:     sdk.ZeroInt(),
			amount1Desired:     sdk.ZeroInt(),
			expectedError:      errors.New("liquidityDelta calculated equals zero"),
		},
		"error: attempt to use and upper and lower tick that are not divisible by tick spacing": {
			tickSpacing:   10,
			expectedError: types.TickSpacingError{TickSpacing: 10, LowerTick: DefaultLowerTick, UpperTick: DefaultUpperTick},
		},
		"error: first position cannot have a zero amount for denom0": {
			amount0Desired: sdk.ZeroInt(),
			expectedError:  types.InitialLiquidityZeroError{Amount0: sdk.ZeroInt(), Amount1: DefaultAmt1},
		},
		"error: first position cannot have a zero amount for denom1": {
			amount1Desired: sdk.ZeroInt(),
			expectedError:  types.InitialLiquidityZeroError{Amount0: DefaultAmt0, Amount1: sdk.ZeroInt()},
		},
		"error: first position cannot have a zero amount for both denom0 and denom1": {
			amount1Desired: sdk.ZeroInt(),
			amount0Desired: sdk.ZeroInt(),
			expectedError:  types.InitialLiquidityZeroError{Amount0: sdk.ZeroInt(), Amount1: sdk.ZeroInt()},
		},
		// TODO: add more tests
		// - custom hand-picked values
		// - think of overflows
		// - think of truncations
	}

	for name, tc := range tests {
		s.Run(name, func() {
			tc := tc
			s.SetupTest()

			// Merge tc with baseCase and update tc to the merged result. This is done to reduce the amount of boilerplate in test cases.
			baseConfigCopy := *baseCase
			mergeConfigs(&baseConfigCopy, &tc)
			tc = baseConfigCopy

			// Fund account to pay for the pool creation fee.
			s.FundAcc(s.TestAccs[0], PoolCreationFee)

			// Create a CL pool with custom tickSpacing
			poolID, err := s.App.SwapRouterKeeper.CreatePool(s.Ctx, clmodel.NewMsgCreateConcentratedPool(s.TestAccs[0], ETH, USDC, tc.tickSpacing))
			s.Require().NoError(err)

			pool, err := s.App.ConcentratedLiquidityKeeper.GetPool(s.Ctx, poolID)
			s.Require().NoError(err)

			var isIncentivized bool
			if len(tc.incentiveIDsCommittedTo) > 0 {
				isIncentivized = true
			}

			// If we want to test a non-first position, we create a first position with a separate account
			if tc.isNotFirstPosition {
				// Fund test account and create the desired position
				s.FundAcc(s.TestAccs[1], sdk.NewCoins(sdk.NewCoin(ETH, DefaultAmt0), sdk.NewCoin(USDC, DefaultAmt1)))
				_, _, _, err := s.App.ConcentratedLiquidityKeeper.CreatePosition(s.Ctx, 1, s.TestAccs[1], DefaultAmt0, DefaultAmt1, sdk.ZeroInt(), sdk.ZeroInt(), tc.lowerTick, tc.upperTick, tc.incentiveIDsCommittedTo)
				s.Require().NoError(err)
			}

			// Fund test account and create the desired position
			s.FundAcc(s.TestAccs[0], sdk.NewCoins(sdk.NewCoin(ETH, DefaultAmt0), sdk.NewCoin(USDC, DefaultAmt1)))

			// Note user and pool account balances before create position is called
			userBalancePrePositionCreation := s.App.BankKeeper.GetAllBalances(s.Ctx, s.TestAccs[0])
			poolBalancePrePositionCreation := s.App.BankKeeper.GetAllBalances(s.Ctx, pool.GetAddress())

			asset0, asset1, liquidityCreated, err := s.App.ConcentratedLiquidityKeeper.CreatePosition(s.Ctx, tc.poolId, s.TestAccs[0], tc.amount0Desired, tc.amount1Desired, tc.amount0Minimum, tc.amount1Minimum, tc.lowerTick, tc.upperTick, tc.incentiveIDsCommittedTo)

			// Note user and pool account balances to compare after create position is called
			userBalancePostPositionCreation := s.App.BankKeeper.GetAllBalances(s.Ctx, s.TestAccs[0])
			poolBalancePostPositionCreation := s.App.BankKeeper.GetAllBalances(s.Ctx, pool.GetAddress())

			// If we expect an error, make sure:
			// - some error was emitted
			// - asset0 and asset1 that were calculated from create position is nil
			// - the error emitted was the expected error
			// - account balances are untouched
			if tc.expectedError != nil {
				s.Require().Error(err)
				s.Require().Equal(asset0, sdk.Int{})
				s.Require().Equal(asset1, sdk.Int{})
				s.Require().ErrorContains(err, tc.expectedError.Error())

				// Check account balances
				s.Require().Equal(userBalancePrePositionCreation.String(), userBalancePostPositionCreation.String())
				s.Require().Equal(poolBalancePrePositionCreation.String(), poolBalancePostPositionCreation.String())

				// Redundantly ensure that position was not created
				position, err := s.App.ConcentratedLiquidityKeeper.GetPosition(s.Ctx, tc.poolId, s.TestAccs[0], tc.lowerTick, tc.upperTick, isIncentivized)
				s.Require().Error(err)
				s.Require().ErrorAs(err, &types.PositionNotFoundError{PoolId: tc.poolId, LowerTick: tc.lowerTick, UpperTick: tc.upperTick})
				s.Require().Nil(position)
				return
			}

			// Else, check that we had no error from creating the position, and that the liquidity and assets that were returned are expected
			s.Require().NoError(err)
			s.Require().Equal(tc.amount0Expected.String(), asset0.String())
			s.Require().Equal(tc.amount1Expected.String(), asset1.String())
			s.Require().Equal(tc.liquidityAmount.String(), liquidityCreated.String())

			// Check account balances
			s.Require().Equal(userBalancePrePositionCreation.Sub(sdk.NewCoins(sdk.NewCoin(ETH, asset0), (sdk.NewCoin(USDC, asset1)))).String(), userBalancePostPositionCreation.String())
			s.Require().Equal(poolBalancePrePositionCreation.Add(sdk.NewCoin(ETH, asset0), (sdk.NewCoin(USDC, asset1))).String(), poolBalancePostPositionCreation.String())

			// Check position state
			s.validatePositionUpdate(s.Ctx, tc.poolId, s.TestAccs[0], tc.lowerTick, tc.upperTick, tc.liquidityAmount, isIncentivized)

			// Check tick state
			s.validateTickUpdates(s.Ctx, tc.poolId, s.TestAccs[0], tc.lowerTick, tc.upperTick, tc.liquidityAmount)
		})
	}
}

func (s *KeeperTestSuite) TestWithdrawPosition() {
	tests := map[string]struct {
		setupConfig *lpTest
		// when this is set, it ovewrites the setupConfig
		// and gives the overwritten configuration to
		// the system under test.
		sutConfigOverwrite           *lpTest
		withdrawIncentivizedPosition []uint64
	}{
		"base case: withdraw full liquidity amount": {
			// setup parameters for creating a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				amount0Expected:         baseCase.amount0Expected, // 0.998587 eth
				amount1Expected:         baseCase.amount1Expected, // 5000 usdc
				incentiveIDsCommittedTo: []uint64{},
			},
			withdrawIncentivizedPosition: []uint64{},
		},
		"withdraw partial liquidity amount": {
			// setup parameters for creating a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				liquidityAmount: baseCase.liquidityAmount.QuoInt64(2),

				amount0Expected:         baseCase.amount0Expected.QuoRaw(2), // 0.4992935 / 2 eth
				amount1Expected:         baseCase.amount1Expected.QuoRaw(2), // 2499 usdc
				incentiveIDsCommittedTo: []uint64{},
			},
			withdrawIncentivizedPosition: []uint64{},
		},
		"error: withdraw position but incentives mismatch": {
			// setup parameters for creation a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				expectedError: types.PositionNotFoundError{PoolId: 1, LowerTick: -1, UpperTick: 86129},
			},
			withdrawIncentivizedPosition: []uint64{},
		},
		"error: no position created": {
			// setup parameters for creation a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				lowerTick:     -1, // valid tick at which no position exists
				expectedError: types.PositionNotFoundError{PoolId: 1, LowerTick: -1, UpperTick: 86129},
			},
			withdrawIncentivizedPosition: []uint64{1},
		},
		"error: pool id for pool that does not exist": {
			// setup parameters for creating a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				poolId:        2, // does not exist
				expectedError: types.PoolNotFoundError{PoolId: 2},
			},
			withdrawIncentivizedPosition: []uint64{1},
		},
		"error: upper tick out of bounds": {
			// setup parameters for creating a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				upperTick:     types.MaxTick + 1, // invalid tick
				expectedError: types.InvalidTickError{Tick: types.MaxTick + 1, IsLower: false},
			},
			withdrawIncentivizedPosition: []uint64{1},
		},
		"error: lower tick out of bounds": {
			// setup parameters for creating a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				lowerTick:     types.MinTick - 1, // invalid tick
				expectedError: types.InvalidTickError{Tick: types.MinTick - 1, IsLower: true},
			},
			withdrawIncentivizedPosition: []uint64{1},
		},
		"error: insufficient liquidity": {
			// setup parameters for creating a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				liquidityAmount: baseCase.liquidityAmount.Add(sdk.OneDec()), // 1 more than available
				expectedError:   types.InsufficientLiquidityError{Actual: baseCase.liquidityAmount.Add(sdk.OneDec()), Available: baseCase.liquidityAmount},
			},
			withdrawIncentivizedPosition: []uint64{1},
		},
		"error: upper tick is below the lower tick, but both are in bounds": {
			// setup parameters for creating a pool and position.
			setupConfig: baseCase,

			// system under test parameters
			// for withdrawing a position.
			sutConfigOverwrite: &lpTest{
				lowerTick:     50,
				upperTick:     40,
				expectedError: types.InvalidLowerUpperTickError{LowerTick: 50, UpperTick: 40},
			},
			withdrawIncentivizedPosition: []uint64{1},
		},
		// TODO: test with custom amounts that potentially lead to truncations.
	}

	for name, tc := range tests {
		s.Run(name, func() {
			// Setup.
			s.SetupTest()

			var (
				ctx                         = s.Ctx
				concentratedLiquidityKeeper = s.App.ConcentratedLiquidityKeeper
				liquidityCreated            = sdk.ZeroDec()
				owner                       = s.TestAccs[0]
				tc                          = tc
				config                      = *tc.setupConfig
				sutConfigOverwrite          = *tc.sutConfigOverwrite
			)

			// If specific configs are provided in the test case, overwrite the config with those values.
			mergeConfigs(&config, &sutConfigOverwrite)

			// If a setupConfig is provided, use it to create a pool and position.
			if tc.setupConfig != nil {
				s.PrepareConcentratedPool()
				var err error
				s.FundAcc(s.TestAccs[0], sdk.NewCoins(sdk.NewCoin("eth", sdk.NewInt(10000000000000)), sdk.NewCoin("usdc", sdk.NewInt(1000000000000))))
				_, _, liquidityCreated, err = concentratedLiquidityKeeper.CreatePosition(ctx, config.poolId, owner, config.amount0Desired, config.amount1Desired, sdk.ZeroInt(), sdk.ZeroInt(), config.lowerTick, config.upperTick, config.incentiveIDsCommittedTo)
				s.Require().NoError(err)
			}

			// System under test.
			amtDenom0, amtDenom1, err := concentratedLiquidityKeeper.WithdrawPosition(ctx, config.poolId, owner, config.lowerTick, config.upperTick, config.liquidityAmount, tc.withdrawIncentivizedPosition)

			if config.expectedError != nil {
				s.Require().Error(err)
				s.Require().Equal(amtDenom0, sdk.Int{})
				s.Require().Equal(amtDenom1, sdk.Int{})
				s.Require().ErrorAs(err, &config.expectedError)
				return
			}

			s.Require().NoError(err)
			s.Require().Equal(config.amount0Expected.String(), amtDenom0.String())
			s.Require().Equal(config.amount1Expected.String(), amtDenom1.String())

			// Determine the liquidity expected to remain after the withdraw.
			expectedRemainingLiquidity := liquidityCreated.Sub(config.liquidityAmount)

			var isIncentivized bool
			if len(config.incentiveIDsCommittedTo) > 0 {
				isIncentivized = true
			}

			// Check that the position was updated.
			s.validatePositionUpdate(ctx, config.poolId, owner, config.lowerTick, config.upperTick, expectedRemainingLiquidity, isIncentivized)

			// check tick state
			// TODO: add is incentivized check
			s.validateTickUpdates(ctx, config.poolId, owner, config.lowerTick, config.upperTick, expectedRemainingLiquidity)
		})
	}
}

// mergeConfigs merges every desired non-zero field from overwrite
// into dst. dst is mutated due to being a pointer.
func mergeConfigs(dst *lpTest, overwrite *lpTest) {
	if overwrite != nil {
		if overwrite.poolId != 0 {
			dst.poolId = overwrite.poolId
		}
		if overwrite.lowerTick != 0 {
			dst.lowerTick = overwrite.lowerTick
		}
		if overwrite.upperTick != 0 {
			dst.upperTick = overwrite.upperTick
		}
		if !overwrite.liquidityAmount.IsNil() {
			dst.liquidityAmount = overwrite.liquidityAmount
		}
		if !overwrite.amount0Minimum.IsNil() {
			dst.amount0Minimum = overwrite.amount0Minimum
		}
		if !overwrite.amount0Desired.IsNil() {
			dst.amount0Desired = overwrite.amount0Desired
		}
		if !overwrite.amount0Expected.IsNil() {
			dst.amount0Expected = overwrite.amount0Expected
		}
		if !overwrite.amount1Minimum.IsNil() {
			dst.amount1Minimum = overwrite.amount1Minimum
		}
		if !overwrite.amount1Desired.IsNil() {
			dst.amount1Desired = overwrite.amount1Desired
		}
		if !overwrite.amount1Expected.IsNil() {
			dst.amount1Expected = overwrite.amount1Expected
		}
		if overwrite.expectedError != nil {
			dst.expectedError = overwrite.expectedError
		}
		if overwrite.tickSpacing != 0 {
			dst.tickSpacing = overwrite.tickSpacing
		}
		if overwrite.isNotFirstPosition != false {
			dst.isNotFirstPosition = overwrite.isNotFirstPosition
		}
		if overwrite.incentiveIDsCommittedTo != nil {
			dst.incentiveIDsCommittedTo = overwrite.incentiveIDsCommittedTo
		}
	}
}

func (s *KeeperTestSuite) TestSendCoinsBetweenPoolAndUser() {
	type sendTest struct {
		coin0       sdk.Coin
		coin1       sdk.Coin
		poolToUser  bool
		expectError bool
	}
	tests := map[string]sendTest{
		"asset0 and asset1 are positive, position creation (user to pool)": {
			coin0: sdk.NewCoin("eth", sdk.NewInt(1000000)),
			coin1: sdk.NewCoin("usdc", sdk.NewInt(1000000)),
		},
		"only asset0 is positive, position creation (user to pool)": {
			coin0: sdk.NewCoin("eth", sdk.NewInt(1000000)),
			coin1: sdk.NewCoin("usdc", sdk.NewInt(0)),
		},
		"only asset1 is positive, position creation (user to pool)": {
			coin0: sdk.NewCoin("eth", sdk.NewInt(0)),
			coin1: sdk.NewCoin("usdc", sdk.NewInt(1000000)),
		},
		"only asset0 is greater than sender has, position creation (user to pool)": {
			coin0:       sdk.NewCoin("eth", sdk.NewInt(100000000000000)),
			coin1:       sdk.NewCoin("usdc", sdk.NewInt(1000000)),
			expectError: true,
		},
		"only asset1 is greater than sender has, position creation (user to pool)": {
			coin0:       sdk.NewCoin("eth", sdk.NewInt(1000000)),
			coin1:       sdk.NewCoin("usdc", sdk.NewInt(100000000000000)),
			expectError: true,
		},
		"asset0 and asset1 are positive, withdraw (pool to user)": {
			coin0:      sdk.NewCoin("eth", sdk.NewInt(1000000)),
			coin1:      sdk.NewCoin("usdc", sdk.NewInt(1000000)),
			poolToUser: true,
		},
		"only asset0 is positive, withdraw (pool to user)": {
			coin0:      sdk.NewCoin("eth", sdk.NewInt(1000000)),
			coin1:      sdk.NewCoin("usdc", sdk.NewInt(0)),
			poolToUser: true,
		},
		"only asset1 is positive, withdraw (pool to user)": {
			coin0:      sdk.NewCoin("eth", sdk.NewInt(0)),
			coin1:      sdk.NewCoin("usdc", sdk.NewInt(1000000)),
			poolToUser: true,
		},
		"only asset0 is greater than sender has, withdraw (pool to user)": {
			coin0:       sdk.NewCoin("eth", sdk.NewInt(100000000000000)),
			coin1:       sdk.NewCoin("usdc", sdk.NewInt(1000000)),
			poolToUser:  true,
			expectError: true,
		},
		"only asset1 is greater than sender has, withdraw (pool to user)": {
			coin0:       sdk.NewCoin("eth", sdk.NewInt(1000000)),
			coin1:       sdk.NewCoin("usdc", sdk.NewInt(100000000000000)),
			poolToUser:  true,
			expectError: true,
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			tc := tc
			s.SetupTest()

			// create a CL pool
			s.PrepareConcentratedPool()

			// store pool interface
			poolI, err := s.App.ConcentratedLiquidityKeeper.GetPoolById(s.Ctx, 1)
			s.Require().NoError(err)
			concentratedPool := poolI.(types.ConcentratedPoolExtension)

			// fund pool address and user address
			s.FundAcc(poolI.GetAddress(), sdk.NewCoins(sdk.NewCoin("eth", sdk.NewInt(10000000000000)), sdk.NewCoin("usdc", sdk.NewInt(1000000000000))))
			s.FundAcc(s.TestAccs[0], sdk.NewCoins(sdk.NewCoin("eth", sdk.NewInt(10000000000000)), sdk.NewCoin("usdc", sdk.NewInt(1000000000000))))

			// set sender and receiver based on test case
			sender := s.TestAccs[0]
			receiver := concentratedPool.GetAddress()
			if tc.poolToUser {
				sender = concentratedPool.GetAddress()
				receiver = s.TestAccs[0]
			}

			// store pre send balance of sender and receiver
			preSendBalanceSender := s.App.BankKeeper.GetAllBalances(s.Ctx, sender)
			preSendBalanceReceiver := s.App.BankKeeper.GetAllBalances(s.Ctx, receiver)

			// system under test
			err = s.App.ConcentratedLiquidityKeeper.SendCoinsBetweenPoolAndUser(s.Ctx, concentratedPool.GetToken0(), concentratedPool.GetToken1(), tc.coin0.Amount, tc.coin1.Amount, sender, receiver)

			// store post send balance of sender and receiver
			postSendBalanceSender := s.App.BankKeeper.GetAllBalances(s.Ctx, sender)
			postSendBalanceReceiver := s.App.BankKeeper.GetAllBalances(s.Ctx, receiver)

			// check error if expected
			if tc.expectError {
				s.Require().Error(err)
				return
			}

			// otherwise, ensure balances are added/deducted appropriately
			expectedPostSendBalanceSender := preSendBalanceSender.Sub(sdk.NewCoins(tc.coin0, tc.coin1))
			expectedPostSendBalanceReceiver := preSendBalanceReceiver.Add(tc.coin0, tc.coin1)

			s.Require().NoError(err)
			s.Require().Equal(expectedPostSendBalanceSender.String(), postSendBalanceSender.String())
			s.Require().Equal(expectedPostSendBalanceReceiver.String(), postSendBalanceReceiver.String())
		})
	}
}

func (s *KeeperTestSuite) TestIsInitialPosition() {
	type sendTest struct {
		initialSqrtPrice sdk.Dec
		initialTick      sdk.Int
		expectedResponse bool
	}
	tests := map[string]sendTest{
		"happy path: is initial position": {
			initialSqrtPrice: sdk.ZeroDec(),
			initialTick:      sdk.ZeroInt(),
			expectedResponse: true,
		},
		"happy path: is not initial position": {
			initialSqrtPrice: DefaultCurrSqrtPrice,
			initialTick:      DefaultCurrTick,
			expectedResponse: false,
		},
		"tick is zero but initialSqrtPrice is not, should not be detected as initial potion": {
			initialSqrtPrice: DefaultCurrSqrtPrice,
			initialTick:      sdk.ZeroInt(),
			expectedResponse: false,
		},
		"initialSqrtPrice is zero but tick is not, should not be detected as initial position (should not happen)": {
			initialSqrtPrice: sdk.ZeroDec(),
			initialTick:      DefaultCurrTick,
			expectedResponse: false,
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			// System under test
			if s.App.ConcentratedLiquidityKeeper.IsInitialPosition(tc.initialSqrtPrice, tc.initialTick) {
				// If we expect the response to be true, then we should check that it is true
				s.Require().True(tc.expectedResponse)
			} else {
				// Else, we should check that it is false
				s.Require().False(tc.expectedResponse)
			}
		})
	}
}

func (s *KeeperTestSuite) TestInitializeInitialPosition() {
	type sendTest struct {
		amount0Desired sdk.Int
		amount1Desired sdk.Int
		expectedError  error
	}
	tests := map[string]sendTest{
		"happy path": {
			amount0Desired: DefaultAmt0,
			amount1Desired: DefaultAmt1,
		},
		"error: amount0Desired is zero": {
			amount0Desired: sdk.ZeroInt(),
			amount1Desired: DefaultAmt1,
			expectedError:  types.InitialLiquidityZeroError{Amount0: sdk.ZeroInt(), Amount1: DefaultAmt1},
		},
		"error: amount1Desired is zero": {
			amount0Desired: DefaultAmt0,
			amount1Desired: sdk.ZeroInt(),
			expectedError:  types.InitialLiquidityZeroError{Amount0: DefaultAmt0, Amount1: sdk.ZeroInt()},
		},
		"error: both amount0Desired and amount01Desired is zero": {
			amount0Desired: sdk.ZeroInt(),
			amount1Desired: sdk.ZeroInt(),
			expectedError:  types.InitialLiquidityZeroError{Amount0: sdk.ZeroInt(), Amount1: sdk.ZeroInt()},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			tc := tc
			s.SetupTest()

			// create a CL pool
			pool := s.PrepareConcentratedPool()

			// System under test
			err := s.App.ConcentratedLiquidityKeeper.InitializeInitialPosition(s.Ctx, pool, tc.amount0Desired, tc.amount1Desired)

			if tc.expectedError != nil {
				s.Require().Error(err)
				s.Require().ErrorAs(err, &tc.expectedError)
			} else {
				s.Require().NoError(err)
			}

		})
	}
}

// stub for testing
func (s *KeeperTestSuite) TestSecondsPerLiquidityInside() {
	type sendTest struct {
		lowerTick     int64
		upperTick     int64
		incentiveID   uint64
		expectedError error
	}
	tests := map[string]sendTest{
		"happy path": {
			lowerTick:   85188,
			upperTick:   87407,
			incentiveID: 1,
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			tc := tc
			s.SetupTest()

			// create a CL pool
			s.PrepareConcentratedPoolWithIncentives()

			// Set the block time to 5 seconds after the current time
			now := s.Ctx.BlockTime()
			s.Ctx = s.Ctx.WithBlockTime(now.Add(time.Second * 5))

			// Fund test account and create the desired position
			s.FundAcc(s.TestAccs[0], sdk.NewCoins(sdk.NewCoin(ETH, DefaultAmt0.Mul(sdk.NewInt(2))), sdk.NewCoin(USDC, DefaultAmt1.Mul(sdk.NewInt(2)))))

			// System under test
			_, _, _, err := s.App.ConcentratedLiquidityKeeper.CreatePosition(s.Ctx, 1, s.TestAccs[0], DefaultAmt0, DefaultAmt1, sdk.ZeroInt(), sdk.ZeroInt(), DefaultLowerTick, DefaultUpperTick, DefaultIncentiveIDsCommittedTo)
			_, _, _, err = s.App.ConcentratedLiquidityKeeper.CreatePosition(s.Ctx, 1, s.TestAccs[0], DefaultAmt0, DefaultAmt1, sdk.ZeroInt(), sdk.ZeroInt(), tc.lowerTick, tc.upperTick, DefaultIncentiveIDsCommittedTo)

			positionOneLower, err := s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, DefaultLowerTick)
			s.Require().NoError(err)
			positionOneUpper, err := s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, DefaultUpperTick)
			s.Require().NoError(err)
			positionTwoLower, err := s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, tc.lowerTick)
			s.Require().NoError(err)
			positionTwoUpper, err := s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, tc.upperTick)
			s.Require().NoError(err)

			fmt.Printf("positionOneLower: %v \n", positionOneLower)
			fmt.Printf("positionOneUpper: %v \n", positionOneUpper)
			fmt.Printf("positionTwoLower: %v \n", positionTwoLower)
			fmt.Printf("positionTwoUpper: %v \n", positionTwoUpper)

			secondsPerLiquidityInsideOne, _ := s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, DefaultLowerTick, DefaultUpperTick, tc.incentiveID)
			secondsPerLiquidityInsideTwo, _ := s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, tc.lowerTick, tc.upperTick, tc.incentiveID)

			fmt.Printf("secondsPerLiquidityInsideOne: %v \n", secondsPerLiquidityInsideOne)
			fmt.Printf("secondsPerLiquidityInsideTwo: %v \n", secondsPerLiquidityInsideTwo)

			// Set the block time to 5 seconds after the current time: Total time elapsed is 10 seconds
			now = s.Ctx.BlockTime()
			s.Ctx = s.Ctx.WithBlockTime(now.Add(time.Second * 5))

			// if tick is active we need to calculate how long it has been active for
			secondsPerLiquidityInsideOne, _ = s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, DefaultLowerTick, DefaultUpperTick, tc.incentiveID)
			secondsPerLiquidityInsideTwo, _ = s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, tc.lowerTick, tc.upperTick, tc.incentiveID)

			fmt.Printf("secondsPerLiquidityInsideOne: %v \n", secondsPerLiquidityInsideOne)
			fmt.Printf("secondsPerLiquidityInsideTwo: %v \n", secondsPerLiquidityInsideTwo)

			pool, err := s.App.ConcentratedLiquidityKeeper.GetPoolById(s.Ctx, 1)
			s.Require().NoError(err)
			fmt.Println()
			fmt.Printf("pool: %v \n", pool)

			// Fund test account and create the desired position
			now = s.Ctx.BlockTime()
			s.Ctx = s.Ctx.WithBlockTime(now.Add(time.Second * 5))
			s.FundAcc(s.TestAccs[0], sdk.NewCoins(sdk.NewCoin(ETH, DefaultAmt0), sdk.NewCoin(USDC, DefaultAmt1)))
			s.App.ConcentratedLiquidityKeeper.SwapExactAmountIn(s.Ctx, s.TestAccs[0], pool, sdk.NewCoin(USDC, DefaultAmt1), ETH, sdk.NewInt(0), sdk.NewDec(0))

			positionOneLower, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 84222)
			s.Require().NoError(err)
			positionOneUpper, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 86129)
			s.Require().NoError(err)
			positionTwoLower, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 85188)
			s.Require().NoError(err)
			positionTwoUpper, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 87407)
			s.Require().NoError(err)
			fmt.Println()
			fmt.Printf("positionOneLower: %v \n", positionOneLower)
			fmt.Printf("positionOneUpper: %v \n", positionOneUpper)
			fmt.Printf("positionTwoLower: %v \n", positionTwoLower)
			fmt.Printf("positionTwoUpper: %v \n", positionTwoUpper)

			pool, err = s.App.ConcentratedLiquidityKeeper.GetPoolById(s.Ctx, 1)
			s.Require().NoError(err)
			fmt.Printf("pool: %v \n", pool)

			secondsPerLiquidityInsideOne, _ = s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, DefaultLowerTick, DefaultUpperTick, tc.incentiveID)
			secondsPerLiquidityInsideTwo, _ = s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, tc.lowerTick, tc.upperTick, tc.incentiveID)

			fmt.Printf("secondsPerLiquidityInsideOne: %v \n", secondsPerLiquidityInsideOne)
			fmt.Printf("secondsPerLiquidityInsideTwo: %v \n", secondsPerLiquidityInsideTwo)

			// Fund test account and create the desired position
			now = s.Ctx.BlockTime()
			s.Ctx = s.Ctx.WithBlockTime(now.Add(time.Second * 5))
			s.FundAcc(s.TestAccs[0], sdk.NewCoins(sdk.NewCoin(ETH, DefaultAmt0), sdk.NewCoin(USDC, DefaultAmt1)))
			s.App.ConcentratedLiquidityKeeper.SwapExactAmountIn(s.Ctx, s.TestAccs[0], pool, sdk.NewCoin(USDC, DefaultAmt1), ETH, sdk.NewInt(0), sdk.NewDec(0))

			positionOneLower, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 84222)
			s.Require().NoError(err)
			positionOneUpper, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 86129)
			s.Require().NoError(err)
			positionTwoLower, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 85188)
			s.Require().NoError(err)
			positionTwoUpper, err = s.App.ConcentratedLiquidityKeeper.GetTickInfo(s.Ctx, 1, 87407)
			s.Require().NoError(err)
			fmt.Println()
			fmt.Printf("positionOneLower: %v \n", positionOneLower)
			fmt.Printf("positionOneUpper: %v \n", positionOneUpper)
			fmt.Printf("positionTwoLower: %v \n", positionTwoLower)
			fmt.Printf("positionTwoUpper: %v \n", positionTwoUpper)

			pool, err = s.App.ConcentratedLiquidityKeeper.GetPoolById(s.Ctx, 1)
			s.Require().NoError(err)
			fmt.Printf("pool: %v \n", pool)

			secondsPerLiquidityInsideOne, _ = s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, DefaultLowerTick, DefaultUpperTick, tc.incentiveID)
			secondsPerLiquidityInsideTwo, _ = s.App.ConcentratedLiquidityKeeper.GetSecondsPerLiquidityInside(s.Ctx, 1, tc.lowerTick, tc.upperTick, tc.incentiveID)

			fmt.Printf("secondsPerLiquidityInsideOne: %v \n", secondsPerLiquidityInsideOne)
			fmt.Printf("secondsPerLiquidityInsideTwo: %v \n", secondsPerLiquidityInsideTwo)

			// s.Require().Equal(1, 3)

			if tc.expectedError != nil {
				//err = fmt.Errorf("test")
				s.Require().Error(err)
			} else {
				//err = fmt.Errorf("test")
				s.Require().NoError(err)
			}

		})
	}
}
