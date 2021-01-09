package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/adshao/go-binance/v2"
	"math"
	"strconv"
	"strings"
)

func PrintPushCoins(client *binance.Client) {
	symbols, err := client.NewListPriceChangeStatsService().Do(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	btcPrice := BTCEURPrice(client)
	fmt.Printf("Symbol,PriceChangePercent,AvgPrice,AvgPriceEUR,Volume,VolumeEUR\n")
	for _, s := range symbols {
		volume := FromS(s.Volume)
		priceAVGEUR := FromS(s.WeightedAvgPrice).Mult(btcPrice)
		volumeEUR := FromS(s.WeightedAvgPrice).Mult(volume).Mult(btcPrice)
		if strings.HasSuffix(s.Symbol, "BTC") &&
			volumeEUR.V > 50000 && volumeEUR.V < 4000000 &&
			priceAVGEUR.V < 1 {
			fmt.Printf("%v,%v,%v,%v,%v,%v\n", s.Symbol, s.PriceChangePercent, s.WeightedAvgPrice, priceAVGEUR, volume, volumeEUR)
		}
	}
}

func BTCEURPrice(client *binance.Client) F {
	return Price(client, "BTCEUR")
}

func Balance(client *binance.Client, symbol string) (free F, locked F) {
	account, err := client.NewGetAccountService().Do(context.Background())
	if err != nil {
		return FromError(fmt.Errorf("could not fetch account info")), FromError(fmt.Errorf("could not fetch account info"))
	}
	for _, b := range account.Balances {
		if b.Asset == symbol || b.Asset+"BTC" == symbol {
			return FromS(b.Free), FromS(b.Locked)
		}
	}
	return F{}, F{}
}

func AvgPrice(client *binance.Client, s string) F {
	price, err := client.NewAveragePriceService().Symbol(s).Do(context.Background())
	if err != nil {
		return FromError(fmt.Errorf("could not fetch average %v price: %w", s, err))
	}

	return FromS(price.Price)
}

func Price(client *binance.Client, s string) F {
	prices, err := client.NewListPricesService().Symbol(s).Do(context.Background())
	if err != nil {
		return FromError(fmt.Errorf("could not fetch %v price: %w", s, err))
	}

	for _, p := range prices {
		return FromS(p.Price)
	}
	return FromError(fmt.Errorf("got no value for %v price", s))
}

func sToF(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

type F struct {
	V   float64
	Err error
}

func (f F) Sub(f2 F) F {
	if !f.Valid() {
		return f
	}
	if !f2.Valid() {
		return f2
	}
	return F{
		V: f.V - f2.V,
	}
}

func (f F) Add(f2 F) F {
	if !f.Valid() {
		return f
	}
	if !f2.Valid() {
		return f2
	}
	return F{
		V: f.V + f2.V,
	}
}

func (f F) Mult(f2 F) F {
	if !f.Valid() {
		return f
	}
	if !f2.Valid() {
		return f2
	}
	return F{
		V: f.V * f2.V,
	}
}

func (f F) Div(f2 F) F {
	if !f.Valid() {
		return f
	}
	if !f2.Valid() {
		return f2
	}
	if f2.V == 0 {
		return F{
			Err: errors.New("division by zero"),
		}
	}
	return F{
		V: f.V / f2.V,
	}
}

func (f F) Floor() F {
	if !f.Valid() {
		return f
	}
	return F{
		V: math.Floor(f.V),
	}
}

func (f F) Valid() bool {
	return f.Err == nil
}

func (f F) String() string {
	if f.Valid() {
		return fmt.Sprintf("%0.8f", f.V)
	}
	return fmt.Sprintf("%v", f.Err)
}

func (f F) StringCompact() string {
	if f.Valid() {
		return strconv.FormatFloat(f.V, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", f.Err)
}

func (f F) StringPrice() string {
	if f.Valid() {
		return strconv.FormatFloat(f.V, 'f', 8, 64)
	}
	return fmt.Sprintf("%v", f.Err)
}

func (f F) StringInt() string {
	if f.Valid() {
		return fmt.Sprintf("%0.0f", f.V)
	}
	return fmt.Sprintf("%v", f.Err)
}

func (f F) FormatPercent() string {
	if f.Valid() {
		return fmt.Sprintf("%0.2f%%", f.V*100)
	}
	return fmt.Sprintf("%v", f.Err)
}

func (f F) FormatEUR() string {
	if f.Valid() {
		return fmt.Sprintf("%0.2f€", f.V)
	}
	return fmt.Sprintf("%v", f.Err)
}

func FromError(err error) F {
	return F{
		Err: err,
	}
}

func FromS(s string) F {
	f, err := strconv.ParseFloat(s, 64)
	if err == nil {
		return F{
			V: f,
		}
	}
	return F{
		Err: err,
	}
}

func FromF(f float64) F {
	return F{
		V: f,
	}
}

func FromI(i int) F {
	return F{
		V: float64(i),
	}
}
