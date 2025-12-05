package main

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"
)

// BertTokenizer handles WordPiece tokenization for BERT-style models
type BertTokenizer struct {
	vocab        map[string]int
	inverseVocab map[int]string
	maxLength    int
	clsTokenID   int
	sepTokenID   int
	padTokenID   int
	unkTokenID   int
}

// TokenizerJSON represents the HuggingFace tokenizer.json format
type TokenizerJSON struct {
	Model struct {
		Vocab map[string]int `json:"vocab"`
	} `json:"model"`
	AddedTokens []struct {
		ID      int    `json:"id"`
		Content string `json:"content"`
	} `json:"added_tokens"`
}

// NewBertTokenizer loads a tokenizer from a HuggingFace tokenizer.json file
func NewBertTokenizer(tokenizerPath string, maxLength int) (*BertTokenizer, error) {
	data, err := os.ReadFile(tokenizerPath)
	if err != nil {
		return nil, err
	}

	var tj TokenizerJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, err
	}

	// Build vocabulary
	vocab := make(map[string]int)
	inverseVocab := make(map[int]string)

	// Add vocabulary from model
	for token, id := range tj.Model.Vocab {
		vocab[token] = id
		inverseVocab[id] = token
	}

	// Add special tokens
	for _, at := range tj.AddedTokens {
		vocab[at.Content] = at.ID
		inverseVocab[at.ID] = at.Content
	}

	// Find special token IDs
	clsID := vocab["[CLS]"]
	sepID := vocab["[SEP]"]
	padID := vocab["[PAD]"]
	unkID := vocab["[UNK]"]

	return &BertTokenizer{
		vocab:        vocab,
		inverseVocab: inverseVocab,
		maxLength:    maxLength,
		clsTokenID:   clsID,
		sepTokenID:   sepID,
		padTokenID:   padID,
		unkTokenID:   unkID,
	}, nil
}

// Encode tokenizes text and returns input_ids and attention_mask
func (t *BertTokenizer) Encode(text string) (inputIDs []int64, attentionMask []int64) {
	// Basic preprocessing
	text = strings.ToLower(text)
	text = strings.TrimSpace(text)

	// Tokenize using basic WordPiece
	tokens := t.tokenize(text)

	// Truncate to max length (accounting for [CLS] and [SEP])
	maxTokens := t.maxLength - 2
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}

	// Build input_ids: [CLS] + tokens + [SEP] + [PAD]...
	inputIDs = make([]int64, t.maxLength)
	attentionMask = make([]int64, t.maxLength)

	inputIDs[0] = int64(t.clsTokenID)
	attentionMask[0] = 1

	for i, token := range tokens {
		inputIDs[i+1] = int64(t.vocabLookup(token))
		attentionMask[i+1] = 1
	}

	inputIDs[len(tokens)+1] = int64(t.sepTokenID)
	attentionMask[len(tokens)+1] = 1

	// Rest is already padded with 0s (PAD token ID is typically 0)
	for i := len(tokens) + 2; i < t.maxLength; i++ {
		inputIDs[i] = int64(t.padTokenID)
		attentionMask[i] = 0
	}

	return inputIDs, attentionMask
}

// tokenize performs basic WordPiece tokenization
func (t *BertTokenizer) tokenize(text string) []string {
	var tokens []string

	// Split on whitespace and punctuation
	words := t.splitWords(text)

	for _, word := range words {
		// Try to find the word in vocab
		if _, ok := t.vocab[word]; ok {
			tokens = append(tokens, word)
			continue
		}

		// WordPiece: break into subwords
		subTokens := t.wordPieceTokenize(word)
		tokens = append(tokens, subTokens...)
	}

	return tokens
}

// splitWords splits text into words, separating punctuation
func (t *BertTokenizer) splitWords(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		} else if unicode.IsPunct(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, string(r))
		} else {
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

// wordPieceTokenize breaks a word into WordPiece tokens
func (t *BertTokenizer) wordPieceTokenize(word string) []string {
	if len(word) == 0 {
		return nil
	}

	var tokens []string
	start := 0
	wordRunes := []rune(word)

	for start < len(wordRunes) {
		end := len(wordRunes)
		found := false

		for end > start {
			substr := string(wordRunes[start:end])
			if start > 0 {
				substr = "##" + substr
			}

			if _, ok := t.vocab[substr]; ok {
				tokens = append(tokens, substr)
				found = true
				break
			}
			end--
		}

		if !found {
			// Unknown character, use [UNK] for the entire remaining word
			tokens = append(tokens, "[UNK]")
			break
		}
		start = end
	}

	return tokens
}

// vocabLookup returns the token ID, or [UNK] ID if not found
func (t *BertTokenizer) vocabLookup(token string) int {
	if id, ok := t.vocab[token]; ok {
		return id
	}
	return t.unkTokenID
}

// VocabSize returns the vocabulary size
func (t *BertTokenizer) VocabSize() int {
	return len(t.vocab)
}
