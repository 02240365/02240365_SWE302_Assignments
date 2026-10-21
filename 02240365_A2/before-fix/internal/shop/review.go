package shop

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const MaxCommentLength = 200

// Review is one customer's rating and comment for one product.
type Review struct {
	ReviewID     string
	ProductID    string
	CustomerID   string
	CustomerName string
	Rating       int32
	Comment      string
	CreatedAt    string
}

// ReviewList is the reviews for one product plus the summary numbers.
type ReviewList struct {
	ProductID     string
	Reviews       []Review
	AverageRating float64
	ReviewCount   int32
}

// Reviews stores reviews in memory.
type Reviews struct {
	mu      sync.Mutex
	byProd  map[string][]Review
	counter int
	now     func() time.Time
}

func NewReviews() *Reviews { return &Reviews{byProd: map[string][]Review{}, now: time.Now} }

// Add records a review. Rules: rating 1 to 5; a comment of 1 to 200
// characters (spaces at the ends are ignored); one review per customer
// per product.
func (r *Reviews) Add(productID, customerID, customerName string, rating int32, comment string) (Review, error) {
	productID, customerID = strings.TrimSpace(productID), strings.TrimSpace(customerID)
	comment = strings.TrimSpace(comment)
	if productID == "" || customerID == "" {
		return Review{}, invalid("product_id and customer_id are required")
	}
	if rating < 1 || rating > 5 {
		return Review{}, invalid("rating must be a whole number from 1 to 5")
	}
	if comment == "" {
		return Review{}, invalid("please write a comment")
	}
	if n := utf8.RuneCountInString(comment); n > MaxCommentLength {
		return Review{}, invalid("the comment is too long: %d characters (the limit is %d)", n, MaxCommentLength)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.byProd[productID] {
		if existing.CustomerID == customerID {
			return Review{}, exists("customer %s has already reviewed product %s", customerID, productID)
		}
	}
	r.counter++
	rev := Review{
		ReviewID: fmt.Sprintf("REV-%04d", r.counter), ProductID: productID, CustomerID: customerID,
		CustomerName: customerName, Rating: rating, Comment: comment,
		CreatedAt: r.now().UTC().Format(time.RFC3339),
	}
	r.byProd[productID] = append(r.byProd[productID], rev)
	return rev, nil
}

// List returns every review of a product, newest first, with the average.
func (r *Reviews) List(productID string) (ReviewList, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return ReviewList{}, invalid("product_id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	list := ReviewList{ProductID: productID, Reviews: []Review{}}
	all := r.byProd[productID]
	sum := int32(0)
	for i := len(all) - 1; i >= 0; i-- {
		list.Reviews = append(list.Reviews, all[i])
		sum += all[i].Rating
	}
	list.ReviewCount = int32(len(all))
	if len(all) > 0 {
		list.AverageRating = round2(float64(sum) / float64(len(all)))
	}
	return list, nil
}

func (r *Reviews) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byProd = map[string][]Review{}
	r.counter = 0
}
