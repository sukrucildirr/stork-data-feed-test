package main

import (
        "context"
        "encoding/hex"
        "encoding/json"
        "fmt"
        "io/ioutil"
        "math/big"
        "os"
        "time"

        bin "github.com/gagliardetto/binary"
        "github.com/gagliardetto/solana-go"
        "github.com/gagliardetto/solana-go/rpc"
        "github.com/rs/zerolog"
        "github.com/rs/zerolog/log"
)

type Config struct {
        RpcUrl              string  `json:"rpcUrl"`
        WsUrl               string  `json:"wsUrl"`
        StorkContractAddress string `json:"storkContractAddress"`
        UpdateFrequency     string  `json:"updateFrequency"`
        Assets              []Asset `json:"assets"`
}

type Asset struct {
        Name          string `json:"name"`
        EncodedAssetId string `json:"encodedAssetId"`
}

type PriceFeed struct {
        Name      string
        Price     *big.Int
        Timestamp uint64
        LastUpdate time.Time
}

type TemporalNumericValue struct {
        TimestampNs    uint64
        QuantizedValue bin.Int128
}

type TemporalNumericValueFeedAccount struct {
        Id          [32]uint8
        LatestValue TemporalNumericValue
}

func main() {
        // Set up logging
        zerolog.TimeFieldFormat = time.RFC3339
        log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

        // Load configuration
        config, err := loadConfig("config.json")
        if err != nil {
                log.Fatal().Err(err).Msg("Failed to load configuration")
        }

        // Create RPC client
        client := rpc.New(config.RpcUrl)

        // Parse Stork contract address
        contractPubKey, err := solana.PublicKeyFromBase58(config.StorkContractAddress)
        if err != nil {
                log.Fatal().Err(err).Msg("Invalid contract address")
        }

        // Set up price feeds
        priceFeeds := make(map[string]*PriceFeed)
        feedAccounts := make(map[string]solana.PublicKey)

        for _, asset := range config.Assets {
                encodedAssetIdBytes, err := hexStringToByteArray(asset.EncodedAssetId)
                if err != nil {
                        log.Fatal().Err(err).Str("assetId", asset.Name).Msg("Failed to convert encoded asset ID to bytes")
                }

                // Derive PDA for feed account
                feedAccount, _, err := solana.FindProgramAddress(
                        [][]byte{
                                []byte("stork_feed"),
                                encodedAssetIdBytes,
                        },
                        contractPubKey,
                )
                if err != nil {
                        log.Fatal().Err(err).Str("assetId", asset.Name).Msg("Failed to derive PDA for feed account")
                }

                feedAccounts[asset.Name] = feedAccount
                priceFeeds[asset.Name] = &PriceFeed{
                        Name: asset.Name,
                }
        }

        // Parse update frequency
        updateFrequency, err := time.ParseDuration(config.UpdateFrequency)
        if err != nil {
                log.Fatal().Err(err).Msg("Invalid update frequency")
        }

        // Start the update loop
        ctx := context.Background()
        ticker := time.NewTicker(updateFrequency)
        defer ticker.Stop()

        log.Info().Msg("Starting price feed monitor...")
        
        // Do an initial update
        updatePriceFeeds(ctx, client, feedAccounts, priceFeeds)
        displayPrices(priceFeeds)

        for {
                select {
                case <-ticker.C:
                        updatePriceFeeds(ctx, client, feedAccounts, priceFeeds)
                        displayPrices(priceFeeds)
                case <-ctx.Done():
                        return
                }
        }
}

func loadConfig(filePath string) (Config, error) {
        var config Config
        data, err := ioutil.ReadFile(filePath)
        if err != nil {
                return config, err
        }
        err = json.Unmarshal(data, &config)
        return config, err
}

func hexStringToByteArray(s string) ([]byte, error) {
        if len(s) >= 2 && s[0:2] == "0x" {
                s = s[2:]
        }
        return hex.DecodeString(s)
}

func updatePriceFeeds(ctx context.Context, client *rpc.Client, feedAccounts map[string]solana.PublicKey, priceFeeds map[string]*PriceFeed) {
        for name, feedAccount := range feedAccounts {
                accountInfo, err := client.GetAccountInfo(ctx, feedAccount)
                if err != nil {
                        log.Error().Err(err).Str("account", feedAccount.String()).Msg("Failed to get account info")
                        continue
                }

                if accountInfo == nil || len(accountInfo.Value.Data.GetBinary()) == 0 {
                        log.Debug().Str("assetId", name).Msg("No value found")
                        continue
                }

                decoder := bin.NewBorshDecoder(accountInfo.Value.Data.GetBinary())
                account := &TemporalNumericValueFeedAccount{}
                err = account.UnmarshalWithDecoder(decoder)
                if err != nil {
                        log.Error().Err(err).Str("account", feedAccount.String()).Msg("Failed to decode account data")
                        continue
                }

                priceFeed := priceFeeds[name]
                priceFeed.Price = account.LatestValue.QuantizedValue.BigInt()
                priceFeed.Timestamp = account.LatestValue.TimestampNs
                priceFeed.LastUpdate = time.Now()
        }
}

func displayPrices(priceFeeds map[string]*PriceFeed) {
        fmt.Println("\n----- Solana Ecosystem Price Feed -----")
        fmt.Println("Updated at:", time.Now().Format(time.RFC3339))
        fmt.Println("----------------------------------------")
        
        for _, feed := range priceFeeds {
                if feed.Price != nil {
                        // Convert the quantized price to a human-readable format
                        // This is a simplified example - you may need to adjust the decimals based on the asset
                        price := new(big.Float).SetInt(feed.Price)
                        price.Quo(price, big.NewFloat(1e8)) // Assuming 8 decimal places
                        
                        timestamp := time.Unix(0, int64(feed.Timestamp))
                        fmt.Printf("%-10s: $%-12.8f (timestamp: %s)\n", feed.Name, price, timestamp.Format(time.RFC3339))
                } else {
                        fmt.Printf("%-10s: No data available\n", feed.Name)
                }
        }
        fmt.Println("----------------------------------------")
}

// For the UnmarshalWithDecoder method to work
func (obj *TemporalNumericValueFeedAccount) UnmarshalWithDecoder(decoder *bin.Decoder) (err error) {
        // Deserialize `Id`
        err = decoder.Decode(&obj.Id)
        if err != nil {
                return err
        }
        // Deserialize `LatestValue`
        err = decoder.Decode(&obj.LatestValue)
        if err != nil {
                return err
        }
        return nil
}