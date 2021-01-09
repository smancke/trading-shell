package main

import (
	"context"
	"fmt"
	"github.com/adshao/go-binance/v2"
	"github.com/smancke/trading-shell/config"
	"strings"
	"time"
)

type Session struct {
	client        *binance.Client
	in            chan string
	out           chan string
	allPriceStats map[string]*binance.PriceChangeStats
	allSymbols    map[string]*binance.Symbol
	selected      string // the selected symbol
	btcPrice      F
	avg24h        F // average for the last 24 hours
	avgRecent     F // average for the last recent time (e.g. 5 min)
	basePrice     F // the base price for limit calculations
	maxInvestEUR  F // max volume for the trading
	buyMaxMult    F // multiplier for the hightest buy limit, relative to the basePrice
	sellMaxMult   F // multiplier for the hightest sell limit, relative to the basePrice
	sellMinMult   F // multiplier for the lowest sell limt to exit, relative to the basePrice
}

func StartSession(config *config.Config) *Session {
	sess := &Session{
		allPriceStats: make(map[string]*binance.PriceChangeStats),
		allSymbols:    make(map[string]*binance.Symbol),
		client:        binance.NewClient(config.APIKey, config.APISecret),
		in:            make(chan string, 1),
		out:           make(chan string, 1),
		maxInvestEUR:  FromF(50.0),
		buyMaxMult:    FromF(1.2),
		sellMaxMult:   FromF(4.5),
		sellMinMult:   FromF(1),
	}

	ex, err := sess.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		sess.Answer(err.Error())
	} else {
		for _, symbol := range ex.Symbols {
			sess.allSymbols[symbol.Symbol] = &symbol
		}
	}

	stats, err := sess.client.NewListPriceChangeStatsService().Do(context.Background())
	if err != nil {
		sess.Answer(err.Error())
	} else {
		for _, stat := range stats {
			sess.allPriceStats[stat.Symbol] = stat
		}
	}

	go sess.dispatch()
	go sess.Put("config")

	return sess
}

func (sess *Session) Put(in string) {
	sess.in <- in
}

func (sess *Session) Get() string {
	return <-sess.out
}

func (sess *Session) Answer(result string) {
	sess.out <- result
}

func (sess *Session) Answerf(format string, a ...interface{}) {
	sess.out <- fmt.Sprintf(format, a...)
}

func (sess *Session) ShowConfig() {
	sess.Answerf(`   Invest: %v EUR
Buy limit: %v
 Sell Max: %v
 Sell Min: %v
`, sess.maxInvestEUR.StringCompact(), sess.buyMaxMult.FormatPercent(), sess.sellMaxMult.FormatPercent(), sess.sellMinMult.FormatPercent())

	account, err := sess.client.NewGetAccountService().Do(context.Background())
	if err != nil {
		sess.Answerf("ERROR ON FETCHING ACCOUNT INFO: %v", err)
	}
	for _, b := range account.Balances {
		total := FromS(b.Free).Add(FromS(b.Locked))
		if b.Asset == "BTC" {
			eur := BTCEURPrice(sess.client).Mult(total).FormatEUR()
			sess.Answerf(" %v: %v / %v", b.Asset, total, eur)
		} else {
			if total.V > 0 {
				sess.Answerf(" %v: %v", b.Asset, total)
			}
		}
	}
}

func (sess *Session) SymbolInfo() {
	symbolInfo := sess.allSymbols[sess.selected]
	sess.Answerf("symbol info %+v", symbolInfo)
}

func (sess *Session) Init(symbol string) {
	if symbol != "" {
		symbol = strings.ToUpper(symbol)
		stats, exist := sess.allPriceStats[symbol]
		if !exist {
			stats, exist = sess.allPriceStats[symbol+"BTC"]
			if !exist {
				sess.Answerf("SYMBOL NOT FOUND: %q", symbol)
				return
			}
		}
		sess.selected = stats.Symbol
		sess.avgRecent = AvgPrice(sess.client, sess.selected)
		sess.basePrice = sess.avgRecent
		sess.avg24h = FromS(stats.WeightedAvgPrice)
		sess.btcPrice = BTCEURPrice(sess.client)
		if !sess.btcPrice.Valid() {
			sess.Answerf("ERROR ON BTC PRICE UPDATE: %v", sess.btcPrice)
		}
	}
	sess.Info()
}

func (sess *Session) Buy(multS string) {
	if sess.selected == "" {
		sess.Answer("NO SYMBOL SELECTED!")
		return
	}

	mult := sess.buyMaxMult
	if multS != "" {
		mult = FromS(multS)
	}

	limit := sess.basePrice.Mult(mult)
	limitS := sess.basePrice.Mult(mult).StringPrice()
	qty := sess.maxInvestEUR.Div(sess.btcPrice).Div(limit).Floor().StringPrice()

	//sess.Answerf("BUY %v @%v", qty, limit)
	order, err := sess.client.NewCreateOrderService().Symbol(sess.selected).
		Side(binance.SideTypeBuy).Type(binance.OrderTypeLimit).
		TimeInForce(binance.TimeInForceTypeGTC).Quantity(qty).
		Price(limitS).Do(context.Background())
	if err != nil {
		sess.Answerf("ERROR ON ORDER FOR %v of %v: %v", qty, sess.selected, err)
		return
	}
	if order.Status == binance.OrderStatusTypeRejected {
		sess.Answerf("ORDER REJECTED!!!!")
	}
	sess.Answerf("%v [%v of %v@%v (%v%% executed, %v)]", order.Side, order.OrigQuantity, order.Symbol, order.Price, FromS(order.ExecutedQuantity).Div(FromS(order.OrigQuantity)).FormatPercent(), order.Status)
}

func (sess *Session) CancelAllOrders() {
	orders, err := sess.client.NewListOpenOrdersService().Symbol(sess.selected).Do(context.Background())
	if len(orders) > 0 || err != nil {
		_, err := sess.client.NewCancelOpenOrdersService().Symbol(sess.selected).Do(context.Background())
		if err != nil {
			sess.Answerf("ERROR ON CANCEL ORDERS %v", err)
			return
		}
		time.Sleep(time.Millisecond * 200)
	}
}

func (sess *Session) SellAllNow(multS string) {
	if sess.selected == "" {
		sess.Answer("NO SYMBOL SELECTED!")
		return
	}

	sess.CancelAllOrders()

	mult := sess.sellMinMult
	if multS != "" {
		mult = FromS(multS)
	}

	limit := sess.basePrice.Mult(mult)
	free, locked := Balance(sess.client, sess.selected)
	if locked.V != 0 {
		sess.Answerf("WARNING: locked balance of %v not in order!", locked)
	}

	order, err := sess.client.NewCreateOrderService().Symbol(sess.selected).
		Side(binance.SideTypeSell).Type(binance.OrderTypeLimit).
		TimeInForce(binance.TimeInForceTypeGTC).Quantity(free.Floor().String()).
		Price(limit.String()).Do(context.Background())
	if err != nil {
		sess.Answerf("ERROR ON SELL ORDER FOR %v of %v: %v", free, sess.selected, err)
		return
	}
	if order.Status == binance.OrderStatusTypeRejected {
		sess.Answerf("ORDER REJECTED!!!!")
	}
	sess.Answerf("%v [%v of %v@%v (%v executed, %v)]", order.Side, order.OrigQuantity, order.Symbol, order.Price, FromS(order.ExecutedQuantity).Div(FromS(order.OrigQuantity)).FormatPercent(), order.Status)
}

func (sess *Session) SellWall(arg string) {
	if sess.selected == "" {
		sess.Answer("NO SYMBOL SELECTED!")
		return
	}

	sess.CancelAllOrders()
	free, locked := Balance(sess.client, sess.selected)
	if locked.V != 0 {
		sess.Answerf("WARNING: locked balance of %v not in order!", locked)
	}

	maxMult := sess.sellMaxMult
	if arg != "" {
		maxMult = FromS(arg)
	}

	steps := 4
	qty := free.Div(FromI(steps)).Floor()
	maxLimit := sess.basePrice.Mult(maxMult)
	deltaPerStep := maxLimit.Sub(sess.basePrice).Div(FromI(steps))
	for step := steps; step > 0; step-- {
		limit := sess.basePrice.Add(deltaPerStep.Mult(FromI(step)))

		order, err := sess.client.NewCreateOrderService().Symbol(sess.selected).
			Side(binance.SideTypeSell).Type(binance.OrderTypeLimit).
			TimeInForce(binance.TimeInForceTypeGTC).Quantity(qty.String()).
			Price(limit.String()).Do(context.Background())
		if err != nil {
			sess.Answerf("ERROR ON SELL ORDER FOR %v of %v: %v", free, sess.selected, err)
			return
		}
		if order.Status == binance.OrderStatusTypeRejected {
			sess.Answerf("ORDER REJECTED!!!!")
		}
		sess.Answerf("%v [%v of %v@%v (%v executed, %v)]", order.Side, order.OrigQuantity, order.Symbol, order.Price, FromS(order.ExecutedQuantity).Div(FromS(order.OrigQuantity)).FormatPercent(), order.Status)
	}
}

func (sess *Session) Price(symbol string) {
	p := Price(sess.client, sess.selected)
	percent := p.Sub(sess.basePrice).Div(sess.basePrice)
	sess.Answerf("price is %v (%v)", p, percent.FormatPercent())
}

func (sess *Session) OrderHistory(showClosed bool) {
	now := time.Now().Unix() * 1000
	trades, err := sess.client.NewListTradesService().Symbol(sess.selected).Limit(10).Do(context.Background())
	if err != nil {
		sess.Answerf("ERROR TRADES ORDERS: %v", err)
	}

	var orders []*binance.Order
	if showClosed {
		orders, err = sess.client.NewListOrdersService().Symbol(sess.selected).Limit(10).Do(context.Background())
	} else {
		orders, err = sess.client.NewListOpenOrdersService().Symbol(sess.selected).Do(context.Background())
	}

	if err != nil {
		sess.Answerf("ERROR LIST ORDERS: %v", err)
	}

	currentPrice := Price(sess.client, sess.selected)
	for i := len(orders) - 1; i >= 0; i-- {
		order := orders[i]
		if showClosed || order.Status == binance.OrderStatusTypeNew || order.Status == binance.OrderStatusTypePartiallyFilled {
			distance := ""
			if order.Status == binance.OrderStatusTypeNew || order.Status == binance.OrderStatusTypePartiallyFilled {
				d := FromS(order.Price).Sub(currentPrice).Div(currentPrice)
				if order.Side == binance.SideTypeSell && d.V < 0 {
					d = d.Mult(FromF(-1))
				}
				if d.V > 0 {
					distance = "-->" + d.FormatPercent()
				}
			}
			sess.Answerf("%v %v, @%v (%v %v) %v", order.Side, FromS(order.OrigQuantity).StringCompact(), order.Price, FromS(order.ExecutedQuantity).Div(FromS(order.OrigQuantity)).FormatPercent(), order.Status, distance)
			for _, trade := range trades {
				if trade.OrderID == order.OrderID {
					sess.Answerf("  -> %v: %v of @%v", time.Millisecond*time.Duration(now-trade.Time), FromS(trade.Quantity).StringCompact(), trade.Price)
				}
			}
		}
	}
}

func (sess *Session) Info() {
	if sess.selected == "" {
		sess.Answer("NO SYMBOL SELECTED!")
		return
	}
	sess.Answerf("\n------ %v --------\n", sess.selected)
	p := Price(sess.client, sess.selected)
	percentCurrentPrice := p.Sub(sess.basePrice).Div(sess.basePrice)
	sess.Answerf("    price: %v (%v)", p, percentCurrentPrice.FormatPercent())
	percentBasePrice := sess.basePrice.Sub(sess.avg24h).Div(sess.avg24h)
	sess.Answerf("basePrice: %v (%v)", sess.basePrice, percentBasePrice.FormatPercent())
	sess.Answerf("  24h AVG: %v\n", sess.avg24h)

	free, locked := Balance(sess.client, sess.selected)
	sess.Answerf("    total: %v", free.Add(locked).StringCompact())
	sess.Answerf("     free: %v", free.StringCompact())
	sess.Answerf("   locked: %v\n", locked.StringCompact())

	sess.OrderHistory(false)
}

func (sess *Session) dispatch() {
	for line := range sess.in {
		pairs := strings.SplitN(line, " ", 2)
		arg := ""
		if len(pairs) > 1 {
			arg = pairs[1]
		}
		switch cmd := pairs[0]; cmd {
		case "config":
			sess.Answerf("\n-------- config ----------")
			sess.ShowConfig()
		case "cancel", "c":
			sess.Answerf("\n-------- cancel ----------")
			sess.CancelAllOrders()
			sess.Info()
		case "price", "p":
			sess.Answerf("\n-------- price -----------")
			sess.Price(arg)
		case "buy", "b":
			sess.Answerf("\n-------- buy  ------------")
			sess.Buy(arg)
		case "sell", "s":
			sess.Answerf("\n-------- sell ------------")
			sess.SellAllNow(arg)
		case "sell-wall", "sw":
			sess.Answerf("\n-------- sell wall----------")
			sess.SellWall(arg)
		case "", "init", "i":
			sess.Init(arg)
		case "history", "h":
			sess.Answerf("\n-------- history ---------")
			sess.OrderHistory(true)
		case "symbol-info":
			sess.Answerf("\n-------- symbol info ---------")
			sess.SymbolInfo()
		case "set invest":

		default:
			if sess.selected == "" {
				sess.Init(cmd)
			} else {
				sess.Answer(fmt.Sprintf("not a command: %v", line))
			}
		}
	}
}
