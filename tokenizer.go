// Package tokenizer implements a data tokenization service.
package tokenizer

import (
	"encoding/base64"
	"errors"
	"github.com/jmcvetta/goutil"
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
	"log"
	"strconv"
	"time"
)

// A TokenNotFound error is returned by GetOriginal if the supplied token 
// string cannot be found in the database.
var TokenNotFound = errors.New("Token Not Found")

// An item of text to be tokenzied, and a channel on a which
// to return the token.
type OriginalText struct {
	text    string      // The original text
	replyto chan string // Channel on which to return token
}

// TokenRecord represents a token in the database.
type TokenRecord struct {
	Text  string // The original text
	Token string // A token representing, but not programmatically derived from, the original text
}

type Tokenizer interface {
	Tokenize(string) string
	Detokenize(string) (string, error)
}

// MongoTokenizer allows you to tokenize and detokenize strings.
type mongoTokenizer struct {
	db *mgo.Database
	// queue   chan OriginalText
}

// The MongoDB collection object containing our tokens.
func (t mongoTokenizer) collection() *mgo.Collection {
	// lightweight operation, involves no network communication
	col := t.db.C("tokens")
	return col
}

// Fetches the token for string s from the database.
func (t mongoTokenizer) fetchToken(s string) (string, error) {
	log.Println("fetchToken:", s)
	var token string
	col := t.collection()
	result := TokenRecord{}
	err := col.Find(bson.M{"original": s}).One(&result)
	if err == nil {
		token = result.Token
	}
	return token, err
}

// Tokenize accepts a string and returns a token string which represents, 
// but has no programmatic relationship to, the original string.
func (t mongoTokenizer) Tokenize(s string) string {
	log.Println("Tokenize:", s)
	var token string
	col := t.collection()
	for {
		var err error
		// 
		// First check for an existing token
		//
		token, err = t.fetchToken(s)
		if err == nil {
			log.Println("Existing token:", token)
			break
		}
		if err != mgo.NotFound {
			// NotFound is harmless - anything else is WTF
			log.Panic(err)
		}
		log.Println("No existing token.")
		//
		// No existing token found, so generate a new token
		//
		// We generate a token that is probably, but not guaranteed to be, 
		// unique by concatenating a string representation of the current 
		// time with a fully random alphanumeric string.
		n := time.Now().Nanosecond()
		token = strconv.Itoa(n)
		token += goutil.RandAlphanumeric(8, 8)
		token = base64.StdEncoding.EncodeToString([]byte(token))
		trec := TokenRecord{
			Text:  s,
			Token: token,
		}
		log.Println(trec)
		err = col.Insert(&trec)
		if err == nil {
			// Success!
			log.Println("New token:", token)
			break
		}
		if e, ok := err.(*mgo.LastError); ok && e.Code == 11000 {
			// MongoDB error code 11000 = duplicate key error
			// Either the token or the original are already in the DB, 
			// possibly put there by a different tokenizer process.
			// 
			// It would probably be better to interpret the text of the
			// Mongo error message to find out which field is a duplicate.
			// For now, we are just going to try fetchToken() for our string,
			// and if that fails try a new token.
			log.Println("Duplicate key")
			log.Println(e)
			continue
		}
		log.Panic(err)
	}
	return token
}

func (t mongoTokenizer) Detokenize(s string) (string, error) {
	log.Println("Detokenize:", s)
	log.Println("  Token:      " + s)
	var orig string
	var err error
	col := t.collection()
	result := TokenRecord{}
	query := col.Find(bson.M{"token": s})
	switch db_err := query.One(&result); true {
	case db_err == mgo.NotFound:
		log.Println("Token not found in DB")
		err = TokenNotFound
		return orig, err
	case db_err != nil:
		log.Panic(err)
	}
	log.Println(result)
	orig = result.Text
	log.Println("Found original for token: " + orig)
	return orig, err
}

func NewMongoTokenizer(db *mgo.Database) Tokenizer {
	//
	// Setup database.  If DB is already setup, this is a noop.
	//
	col := db.C("tokens")
	col.EnsureIndex(mgo.Index{
		Key:      []string{"original"},
		Unique:   true,
		DropDups: false,
		Sparse:   true,
	})
	col.EnsureIndex(mgo.Index{
		Key:      []string{"token"},
		Unique:   true,
		DropDups: false,
		Sparse:   true,
	})
	//
	// Initialize tokenizer
	//
	t := mongoTokenizer{
		db: db,
	}
	return t
}