package discord

import (
	"fmt"
	"log"
	"pactus-bot/client"
	"pactus-bot/config"
	"pactus-bot/wallet"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pactus-project/pactus/crypto"
	"github.com/pactus-project/pactus/util"
)

type Bot struct {
	discordSession *discordgo.Session
	faucetWallet   *wallet.Wallet
	cfg            *config.Config
	store          *SafeStore
}

func Start(cfg *config.Config, w *wallet.Wallet, ss *SafeStore) (*Bot, error) {
	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Printf("error creating Discord session: %v", err)
		return nil, err
	}
	bot := &Bot{cfg: cfg, discordSession: dg, faucetWallet: w, store: ss}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(bot.messageHandler)

	// In this example, we only care about receiving message events.
	dg.Identify.Intents = discordgo.IntentsAllWithoutPrivileged

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		log.Printf("error opening connection: %v", err)
		return nil, err
	}
	return bot, nil
}

func (b *Bot) Stop() error {
	return b.discordSession.Close()
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func (b *Bot) messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {

	log.Printf(m.Content)

	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Content == "help" {
		msg := "You can request the faucet by sending your wallet address, e.g tpc1pxl333elgnrdtk0kjpjdvky44yu62x0cwupnpjl"
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}
	if m.Content == "network" {
		msg := b.networkInfo()
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}
	if m.Content == "address" {
		msg := fmt.Sprintf("Faucet address is: %v", b.cfg.FaucetAddress)
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}
	// If the message is "balance" reply with "available faucet balance"
	if m.Content == "balance" {
		b := b.faucetWallet.GetBalance()
		msg := fmt.Sprintf("Available faucet balance is %.6f PAC", b.Available)
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}

	// faucet message must contain address/pubkey
	trimmedAddress := strings.Trim(m.Content, " ")
	peerID, pubKey, isValid, msg := b.validateInfo(trimmedAddress)

	if !isValid {
		s.ChannelMessageSend(m.ChannelID, msg)
		return
	}

	if pubKey != "" {

		//check available balance
		balance := b.faucetWallet.GetBalance()
		if balance.Available < b.cfg.FaucetAmount {
			s.ChannelMessageSend(m.ChannelID, "Insuffcient faucet balance. Try again later.")
			return
		}

		//send faucet
		txHash := b.faucetWallet.BondTransaction(pubKey, trimmedAddress, b.cfg.FaucetAmount)
		if txHash != "" {
			err := b.store.SetData(peerID, trimmedAddress, m.Author.Username, m.Author.ID, b.cfg.FaucetAmount)
			if err != nil {
				log.Printf("error saving faucet information: %v\n", err)
			}
			msg := fmt.Sprintf("Faucet ( %.6f PAC) is staked on node successfully!", b.cfg.FaucetAmount)
			s.ChannelMessageSend(m.ChannelID, msg)
		}
	}
}

func (b *Bot) validateInfo(address string) (string, string, bool, string) {
	_, err := crypto.AddressFromString(address)
	if err != nil {
		log.Printf("invalid address")
		return "", "", false, "Pactus Universal Robot is unable to handle your request. If you are requesting testing faucet, supply the valid address."
	}
	client, err := client.NewClient(b.cfg.Server)
	if err != nil {
		log.Printf("error establishing connection")
		return "", "", false, "The bot cannot establish connection to the blochain network. Try again later."
	}
	defer client.Close()
	peerInfo, pub, err := client.GetPeerInfo(address)
	if err != nil || pub == nil {
		log.Printf("error getting peer info")
		return "", "", false, "Your node information could not obtained. Make sure your node is fully synced before requesting the faucet."
	}

	// check if the validator has already been given the faucet
	peerID, err := peer.IDFromBytes(peerInfo.PeerId)
	if err != nil || peerID.String() == "" {
		log.Printf("error getting peer id")
		return "", "", false, "Your node information could not obtained. Make sure your node is fully synced before requesting the faucet."
	}
	v, exists := b.store.GetData(peerID.String())
	if exists || v != nil {
		return "", "", false, "Sorry. You already received faucet using this address: " + v.ValidatorAddress
	}

	//check block height
	height, err := client.GetBlockchainHeight()
	if err != nil {
		log.Printf("error current block height")
		return "", "", false, "The bot cannot establish connection to the blochain network. Try again later."
	}
	if (height - peerInfo.Height) > 1080 {
		msg := fmt.Sprintf("Your node is not fully synchronised. It is is behind by %v blocks. Make sure that your node is fully synchronised before requesting faucet.", height-peerInfo.Height)

		log.Printf("peer %s with address %v is not well synced: ", peerInfo.PeerId, address)
		return "", "", false, msg
	}
	return peerID.String(), pub.String(), true, ""
}

func (b *Bot) networkInfo() string {
	msg := "Pactus is truely decentralised proof of stake blockcahin."
	client, err := client.NewClient(b.cfg.Server)
	if err != nil {
		log.Printf("error establishing connection")
		return msg
	}
	defer client.Close()
	nodes, err := client.GetNetworkInfo()
	if err != nil {
		log.Printf("error establishing connection")
		return msg
	}
	msg += "\nThe following are the currentl statistics:\n"
	msg += fmt.Sprintf("Network started at : %v\n", time.UnixMilli(nodes.StartedAt*1000).Format("02/01/2006, 15:04:05"))
	msg += fmt.Sprintf("Total bytes sent : %v\n", nodes.TotalSentBytes)
	msg += fmt.Sprintf("Total received bytes : %v\n", nodes.TotalReceivedBytes)
	msg += fmt.Sprintf("Number of peer nodes: %v\n", len(nodes.Peers))
	//check block height
	blochainInfo, err := client.GetBlockchainInfo()
	if err != nil {
		log.Printf("error current block height")
		return msg
	}
	msg += fmt.Sprintf("Block height: %v\n", blochainInfo.LastBlockHeight)
	msg += fmt.Sprintf("Total power: %v PACs\n", util.ChangeToCoin(blochainInfo.TotalPower))
	msg += fmt.Sprintf("Total committee power: %v PACs\n", util.ChangeToCoin(blochainInfo.CommitteePower))
	msg += fmt.Sprintf("Total validators: %v\n", blochainInfo.TotalValidators)
	return msg
}