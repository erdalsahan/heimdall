package cli

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/maticnetwork/bor/common"
	"github.com/maticnetwork/heimdall/bridge/setu/util"
	hmClient "github.com/maticnetwork/heimdall/client"
	"github.com/maticnetwork/heimdall/contracts/stakinginfo"
	"github.com/maticnetwork/heimdall/helper"
	"github.com/maticnetwork/heimdall/staking/types"
	hmTypes "github.com/maticnetwork/heimdall/types"
)

var logger = helper.Logger.With("module", "staking/client/cli")

// GetTxCmd returns the transaction commands for this module
func GetTxCmd(cdc *codec.Codec) *cobra.Command {
	txCmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Staking transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       hmClient.ValidateCmd,
	}

	txCmd.AddCommand(
		client.PostCommands(
			SendValidatorJoinTx(cdc),
			SendValidatorUpdateTx(cdc),
			SendValidatorExitTx(cdc),
			SendValidatorStakeUpdateTx(cdc),
		)...,
	)
	return txCmd
}

// SendValidatorJoinTx send validator join transaction
func SendValidatorJoinTx(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validator-join",
		Short: "Join Heimdall as a validator",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			// get proposer
			proposer := hmTypes.HexToHeimdallAddress(viper.GetString(FlagProposerAddress))
			if proposer.Empty() {
				proposer = helper.GetFromAddress(cliCtx)
			}

			txhash := viper.GetString(FlagTxHash)
			if txhash == "" {
				return fmt.Errorf("transaction hash is required")
			}

			pubkeyStr := viper.GetString(FlagSignerPubkey)
			if pubkeyStr == "" {
				return fmt.Errorf("pubkey is required")
			}

			pubkeyBytes := common.FromHex(pubkeyStr)
			if len(pubkeyBytes) != 65 {
				return fmt.Errorf("Invalid public key length")
			}
			pubkey := hmTypes.NewPubKey(pubkeyBytes)

			contractCallerObj, err := helper.NewContractCaller()
			if err != nil {
				return err
			}

			chainmanagerParams, err := util.GetChainmanagerParams(cliCtx)
			if err != nil {
				return err
			}

			// get main tx receipt
			receipt, err := contractCallerObj.GetConfirmedTxReceipt(time.Now().UTC(), hmTypes.HexToHeimdallHash(txhash).EthHash(), chainmanagerParams.TxConfirmationTime)
			if err != nil || receipt == nil {
				return errors.New("Transaction is not confirmed yet. Please wait for sometime and try again")
			}

			abiObject := &contractCallerObj.StakingInfoABI
			eventName := "Staked"
			event := new(stakinginfo.StakinginfoStaked)
			var logIndex uint
			found := false
			for _, vLog := range receipt.Logs {
				topic := vLog.Topics[0].Bytes()
				selectedEvent := helper.EventByID(abiObject, topic)
				if selectedEvent != nil && selectedEvent.Name == eventName {
					if err := helper.UnpackLog(abiObject, event, eventName, vLog); err != nil {
						return err
					}

					logIndex = vLog.Index
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("Invalid tx for validator join")
			}

			if !bytes.Equal(event.SignerPubkey, pubkey.Bytes()[1:]) {
				return fmt.Errorf("Public key mismatch with event log")
			}

			// msg
			msg := types.NewMsgValidatorJoin(
				proposer,
				event.ValidatorId.Uint64(),
				pubkey,
				hmTypes.HexToHeimdallHash(txhash),
				uint64(logIndex),
				event.Nonce.Uint64(),
			)

			// broadcast messages
			return helper.BroadcastMsgsWithCLI(cliCtx, []sdk.Msg{msg})
		},
	}

	cmd.Flags().StringP(FlagProposerAddress, "p", "", "--proposer=<proposer-address>")
	cmd.Flags().String(FlagSignerPubkey, "", "--signer-pubkey=<signer pubkey here>")
	cmd.Flags().String(FlagTxHash, "", "--tx-hash=<transaction-hash>")
	if err := cmd.MarkFlagRequired(FlagSignerPubkey); err != nil {
		logger.Error("SendValidatorJoinTx | MarkFlagRequired | FlagSignerPubkey", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagTxHash); err != nil {
		logger.Error("SendValidatorJoinTx | MarkFlagRequired | FlagTxHash", "Error", err)
	}
	return cmd
}

// SendValidatorExitTx sends validator exit transaction
func SendValidatorExitTx(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validator-exit",
		Short: "Exit heimdall as a validator ",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			// get proposer
			proposer := hmTypes.HexToHeimdallAddress(viper.GetString(FlagProposerAddress))
			if proposer.Empty() {
				proposer = helper.GetFromAddress(cliCtx)
			}

			validator := viper.GetInt64(FlagValidatorID)
			if validator == 0 {
				return fmt.Errorf("validator ID cannot be 0")
			}

			txhash := viper.GetString(FlagTxHash)
			if txhash == "" {
				return fmt.Errorf("transaction hash has to be supplied")
			}

			nonce := viper.GetInt64(FlagNonce)

			// draf msg
			msg := types.NewMsgValidatorExit(
				proposer,
				uint64(validator),
				hmTypes.HexToHeimdallHash(txhash),
				uint64(viper.GetInt64(FlagLogIndex)),
				uint64(nonce),
			)

			// broadcast messages
			return helper.BroadcastMsgsWithCLI(cliCtx, []sdk.Msg{msg})
		},
	}

	cmd.Flags().StringP(FlagProposerAddress, "p", "", "--proposer=<proposer-address>")
	cmd.Flags().Int(FlagValidatorID, 0, "--id=<validator ID here>")
	cmd.Flags().String(FlagTxHash, "", "--tx-hash=<transaction-hash>")
	cmd.Flags().String(FlagLogIndex, "", "--log-index=<log-index>")
	cmd.Flags().String(FlagNonce, "", "--nonce=<nonce>")
	if err := cmd.MarkFlagRequired(FlagValidatorID); err != nil {
		logger.Error("SendValidatorExitTx | MarkFlagRequired | FlagValidatorID", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagTxHash); err != nil {
		logger.Error("SendValidatorExitTx | MarkFlagRequired | FlagTxHash", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagLogIndex); err != nil {
		logger.Error("SendValidatorExitTx | MarkFlagRequired | FlagLogIndex", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagNonce); err != nil {
		logger.Error("SendValidatorExitTx | MarkFlagRequired | FlagNonce", "Error", err)
	}

	return cmd
}

// SendValidatorUpdateTx send validator update transaction
func SendValidatorUpdateTx(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "signer-update",
		Short: "Update signer for a validator",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			// get proposer
			proposer := hmTypes.HexToHeimdallAddress(viper.GetString(FlagProposerAddress))
			if proposer.Empty() {
				proposer = helper.GetFromAddress(cliCtx)
			}

			validator := viper.GetInt64(FlagValidatorID)
			if validator == 0 {
				return fmt.Errorf("validator ID cannot be 0")
			}

			pubkeyStr := viper.GetString(FlagNewSignerPubkey)
			if pubkeyStr == "" {
				return fmt.Errorf("Pubkey has to be supplied")
			}

			pubkeyBytes, err := hex.DecodeString(pubkeyStr)
			if err != nil {
				return err
			}
			pubkey := hmTypes.NewPubKey(pubkeyBytes)

			txhash := viper.GetString(FlagTxHash)
			if txhash == "" {
				return fmt.Errorf("transaction hash has to be supplied")
			}

			msg := types.NewMsgSignerUpdate(
				proposer,
				uint64(validator),
				pubkey,
				hmTypes.HexToHeimdallHash(txhash),
				uint64(viper.GetInt64(FlagLogIndex)),
				uint64(viper.GetInt64(FlagNonce)),
			)

			// broadcast messages
			return helper.BroadcastMsgsWithCLI(cliCtx, []sdk.Msg{msg})
		},
	}

	cmd.Flags().StringP(FlagProposerAddress, "p", "", "--proposer=<proposer-address>")
	cmd.Flags().Int(FlagValidatorID, 0, "--id=<validator-id>")
	cmd.Flags().String(FlagNewSignerPubkey, "", "--new-pubkey=<new-signer-pubkey>")
	cmd.Flags().String(FlagTxHash, "", "--tx-hash=<transaction-hash>")
	cmd.Flags().String(FlagLogIndex, "", "--log-index=<log-index>")
	if err := cmd.MarkFlagRequired(FlagTxHash); err != nil {
		logger.Error("SendValidatorUpdateTx | MarkFlagRequired | FlagTxHash", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagNewSignerPubkey); err != nil {
		logger.Error("SendValidatorUpdateTx | MarkFlagRequired | FlagNewSignerPubkey", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagLogIndex); err != nil {
		logger.Error("SendValidatorUpdateTx | MarkFlagRequired | FlagLogIndex", "Error", err)
	}

	return cmd
}

// SendValidatorStakeUpdateTx send validator stake update transaction
func SendValidatorStakeUpdateTx(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stake-update",
		Short: "Update stake for a validator",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := context.NewCLIContext().WithCodec(cdc)

			// get proposer
			proposer := hmTypes.HexToHeimdallAddress(viper.GetString(FlagProposerAddress))
			if proposer.Empty() {
				proposer = helper.GetFromAddress(cliCtx)
			}

			validator := viper.GetInt64(FlagValidatorID)
			if validator == 0 {
				return fmt.Errorf("validator ID cannot be 0")
			}

			txhash := viper.GetString(FlagTxHash)
			if txhash == "" {
				return fmt.Errorf("transaction hash has to be supplied")
			}

			msg := types.NewMsgStakeUpdate(
				proposer,
				uint64(validator),
				hmTypes.HexToHeimdallHash(txhash),
				uint64(viper.GetInt64(FlagLogIndex)),
				viper.GetUint64(FlagNonce),
			)

			// broadcast messages
			return helper.BroadcastMsgsWithCLI(cliCtx, []sdk.Msg{msg})
		},
	}

	cmd.Flags().StringP(FlagProposerAddress, "p", "", "--proposer=<proposer-address>")
	cmd.Flags().Int(FlagValidatorID, 0, "--id=<validator-id>")
	cmd.Flags().String(FlagTxHash, "", "--tx-hash=<transaction-hash>")
	cmd.Flags().String(FlagLogIndex, "", "--log-index=<log-index>")
	cmd.Flags().Int(FlagNonce, "", "--nonce=<nonce>")
	if err := cmd.MarkFlagRequired(FlagTxHash); err != nil {
		logger.Error("SendValidatorStakeUpdateTx | MarkFlagRequired | FlagTxHash", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagLogIndex); err != nil {
		logger.Error("SendValidatorStakeUpdateTx | MarkFlagRequired | FlagLogIndex", "Error", err)
	}
	if err := cmd.MarkFlagRequired(FlagNonce); err != nil {
		logger.Error("SendValidatorStakeUpdateTx | MarkFlagRequired | FlagNonce", "Error", err)
	}

	return cmd
}
