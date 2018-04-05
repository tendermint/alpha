package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/tendermint/types"
)

var (
	genesisDocs = make(map[string]types.GenesisDoc)
)

type ErrorGenesisDocNotFound struct {
	chainID string
}

func (e ErrorGenesisDocNotFound) Error() string {
	return fmt.Sprintf("genesis with such chain ID %s not found", e.chainID)
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	for chainID := range genesisDocs {
		fmt.Fprintf(w, "<h1><a href=\"/view/%s\">%s</a></h1>", chainID, chainID)
	}
	fmt.Fprintf(w, "<a href=\"/new\">New</a>")
}

func newHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "<h1>New genesis</h1>"+
		"<form action=\"/create\" method=\"POST\">"+
		"ChainID <input type=\"text\" name=\"chainID\"><br>"+
		"ConsensusParams (not working now) <textarea name=\"consensus_params\"></textarea><br>"+
		"Your Validator PubKey (raw json) (optional) <textarea name=\"your_validator_pubkey\"></textarea><br>"+
		"Your Validator Power (optional) <input type=\"number\" name=\"your_validator_power\"><br>"+
		"Your Validator Name (optional) <input type=\"text\" name=\"your_validator_name\"><br>"+
		"App Hash <input type=\"text\" name=\"app_hash\"><br>"+
		"App State (raw json) <textarea name=\"app_state\"></textarea><br>"+
		"<input type=\"submit\" value=\"Create\">"+
		"</form>")
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	chainID := r.FormValue("chainID")

	// TODO allow changing
	consensusParams := types.DefaultConsensusParams()

	validators := []types.GenesisValidator{}
	if r.FormValue("your_validator_pubkey") != "" && r.FormValue("your_validator_power") != "" && r.FormValue("your_validator_name") != "" {
		power, err := strconv.ParseInt(r.FormValue("your_validator_power"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
			return
		}

		var yourValidatorPubKey crypto.PubKey
		err = json.Unmarshal([]byte(r.FormValue("your_validator_pubkey")), &yourValidatorPubKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		validators = append(validators, types.GenesisValidator{
			PubKey: yourValidatorPubKey,
			Power:  power,
			Name:   r.FormValue("your_validator_name"),
		})
	} else {
		http.Error(w, "incorrect your_validator fields", http.StatusNotAcceptable)
		return
	}

	appHash := r.FormValue("app_hash")
	appState := json.RawMessage([]byte(r.FormValue("app_state")))

	genesisDocs[chainID] = types.GenesisDoc{
		GenesisTime:     time.Now(),
		ChainID:         chainID,
		ConsensusParams: consensusParams,
		Validators:      validators,
		AppHash:         []byte(appHash),
		AppStateJSON:    appState,
	}

	fmt.Fprintf(w, "<h1>Give <a href=\"/new_validator/%s\">this link</a> to other validators && <a href=\"/view/%s\">view genesis JSON</a></h1>", chainID, chainID)
}

var validPath = regexp.MustCompile("^/(new_validator|add_validator|view)/([a-zA-Z0-9]+)$")

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func newValidatorHandler(w http.ResponseWriter, r *http.Request, chainID string) {
	_, ok := genesisDocs[chainID]
	if !ok {
		http.Error(w, ErrorGenesisDocNotFound{chainID}.Error(), http.StatusNotFound)
		return
	}

	fmt.Fprintf(w, "<h1>Add validator</h1>"+
		"<form action=\"/add_validator/%s\" method=\"POST\">"+
		"Your Validator PubKey (raw json) (optional) <textarea name=\"your_validator_pubkey\"></textarea><br>"+
		"Your Validator Power (optional) <input type=\"number\" name=\"your_validator_power\"><br>"+
		"Your Validator Name (optional) <input type=\"text\" name=\"your_validator_name\"><br>"+
		"<input type=\"submit\" value=\"Add\">"+
		"</form>", chainID)
}

func addValidatorHandler(w http.ResponseWriter, r *http.Request, chainID string) {
	genDoc, ok := genesisDocs[chainID]
	if !ok {
		http.Error(w, ErrorGenesisDocNotFound{chainID}.Error(), http.StatusNotFound)
		return
	}

	if r.FormValue("your_validator_pubkey") != "" && r.FormValue("your_validator_power") != "" && r.FormValue("your_validator_name") != "" {
		power, err := strconv.ParseInt(r.FormValue("your_validator_power"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotAcceptable)
			return
		}

		var yourValidatorPubKey crypto.PubKey
		err = json.Unmarshal([]byte(r.FormValue("your_validator_pubkey")), &yourValidatorPubKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		genDoc.Validators = append(genDoc.Validators, types.GenesisValidator{
			PubKey: yourValidatorPubKey,
			Power:  power,
			Name:   r.FormValue("your_validator_name"),
		})
		genesisDocs[chainID] = genDoc
		http.Redirect(w, r, "/view/"+chainID, http.StatusFound)
	} else {
		http.Error(w, "incorrect your_validator fields", http.StatusNotAcceptable)
		return
	}
}

func viewHandler(w http.ResponseWriter, r *http.Request, chainID string) {
	genDoc, ok := genesisDocs[chainID]
	if !ok {
		http.Error(w, ErrorGenesisDocNotFound{chainID}.Error(), http.StatusNotFound)
		return
	}

	bytes, err := json.Marshal(genDoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, string(bytes))
}

func main() {
	http.HandleFunc("/", listHandler)

	http.HandleFunc("/new", newHandler)
	http.HandleFunc("/create", createHandler)

	http.HandleFunc("/new_validator/", makeHandler(newValidatorHandler))
	http.HandleFunc("/add_validator/", makeHandler(addValidatorHandler))
	http.HandleFunc("/view/", makeHandler(viewHandler))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
