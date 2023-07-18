package main

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/DigitalLabs-web3/neo-evm-bridge/relayer/cli/minter/relay"
	"github.com/DigitalLabs-web3/neo-evm-bridge/relayer/config"
	"github.com/DigitalLabs-web3/neo-go-evm/pkg/wallet"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/term"
)

func main() {
	cfg, err := config.Load("config.json")
	if err != nil {
		panic(fmt.Errorf("can't load config: %w", err))
	}
	acc, err := openWallet(cfg.Wallet, common.HexToAddress(cfg.Relayer))
	if err != nil {
		panic(fmt.Errorf("can't open wallet: %w", err))
	}
	relayer, err := relay.NewRelayer(cfg, acc)
	if err != nil {
		panic(fmt.Errorf("can't initialize relayer: %w", err))
	}
	relayer.Run()
}

func openWallet(path string, address common.Address) (*wallet.Account, error) {
	wall, err := wallet.NewWalletFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't open wallet: %w", err)
	}
	if len(wall.Accounts) == 0 {
		return nil, fmt.Errorf("no account in wallet")
	}
	for _, acc := range wall.Accounts {
		if acc.Address == address {
			pass, err := readPassword(address)
			if err != nil {
				return nil, fmt.Errorf("can't read password: %w", err)
			}
			err = acc.Decrypt(pass, wall.Scrypt)
			if err != nil {
				return nil, fmt.Errorf("can't decipher account: %w", err)
			}
			return acc, nil
		}
	}
	return nil, errors.New("relayer not found in wallet")
}

func readPassword(address common.Address) (string, error) {
	fmt.Printf("please enter passowrd for %s:\n", address)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	pass := string(bytePassword)
	return pass, err
}
