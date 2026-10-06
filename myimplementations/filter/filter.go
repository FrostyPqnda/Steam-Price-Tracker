package filter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const FilterHelp = "sale, free, under:<price>, over:<price>, discount:<min%>, name:<text>, prefix:<text>"

// ParseFilter turns "sale,under:10" into a Mongo filter.
func ParseFilter(spec string) (bson.M, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return bson.M{}, nil
	}

	var clauses []bson.M
	for _, part := range strings.Split(spec, ",") {
		key, val, _ := strings.Cut(strings.TrimSpace(part), ":")
		key = strings.ToLower(key)

		num := func() (float64, error) {
			n, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return 0, fmt.Errorf("filter %q needs a number, e.g. %s:10", key, key)
			}
			return n, nil
		}

		switch key {
		case "sale":
			clauses = append(clauses, bson.M{"price_snapshot.discount": bson.M{"$ne": 0}})
		case "free":
			clauses = append(clauses, bson.M{"price_snapshot.discount_price": 0})
		case "under":
			n, err := num()
			if err != nil {
				return nil, err
			}
			// $gt 0 keeps free/unpriced listings out of "under" results
			clauses = append(clauses, bson.M{"price_snapshot.discount_price": bson.M{"$gt": 0, "$lte": n}})
		case "over":
			n, err := num()
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, bson.M{"price_snapshot.discount_price": bson.M{"$gte": n}})
		case "discount":
			n, err := num()
			if err != nil {
				return nil, err
			}
			// $abs makes this work whether discounts are stored as 75 or -75
			clauses = append(clauses, bson.M{"$expr": bson.M{
				"$gte": bson.A{bson.M{"$abs": "$price_snapshot.discount"}, n},
			}})
		case "prefix":
			if val == "" {
				return nil, fmt.Errorf("filter \"prefix\" needs text, e.g. prefix:half")
			}
			clauses = append(clauses, bson.M{"title": bson.M{
				"$regex": "^" + regexp.QuoteMeta(val), "$options": "i",
			}})
		case "name":
			if val == "" {
				return nil, fmt.Errorf("filter \"name\" needs text, e.g. name:dark")
			}
			clauses = append(clauses, bson.M{"title": bson.M{
				"$regex": regexp.QuoteMeta(val), "$options": "i",
			}})
		default:
			return nil, fmt.Errorf("unknown filter %q (available: %s)", key, FilterHelp)
		}
	}

	if len(clauses) == 1 {
		return clauses[0], nil
	}
	return bson.M{"$and": clauses}, nil
}
