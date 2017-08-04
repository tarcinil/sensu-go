package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sensu/sensu-go/backend/authorization"
	"github.com/sensu/sensu-go/backend/store"
	"github.com/sensu/sensu-go/types"
)

// ChecksController defines the fields required by ChecksController.
type ChecksController struct {
	Store store.Store
}

// Register should define an association between HTTP routes and their
// respective handlers defined within this Controller.
func (c *ChecksController) Register(r *mux.Router) {
	r.HandleFunc("/checks", c.many).Methods(http.MethodGet)
	r.HandleFunc("/checks", c.single).Methods(http.MethodPost)
	r.HandleFunc("/checks/{name}", c.single).Methods(http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete)
}

// many handles requests to /checks
func (c *ChecksController) many(w http.ResponseWriter, r *http.Request) {
	abilities := authorization.Checks.WithContext(r.Context())
	if r.Method == http.MethodGet && !abilities.CanList() {
		authorization.UnauthorizedAccessToResource(w)
		return
	}

	checks, err := c.Store.GetCheckConfigs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reject those resources the viewer is unauthorized to view
	rejectChecks(&checks, abilities.CanRead)

	checksBytes, err := json.Marshal(checks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, string(checksBytes))
}

// single handles requests to /checks/:name
func (c *ChecksController) single(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name, _ := vars["name"]
	method := r.Method

	abilities := authorization.Checks.WithContext(r.Context())
	if r.Method == http.MethodDelete && !abilities.CanDelete() {
		authorization.UnauthorizedAccessToResource(w)
		return
	}

	var (
		check *types.CheckConfig
		err   error
	)

	if method == http.MethodGet || method == http.MethodDelete {
		check, err = c.Store.GetCheckConfigByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if check == nil {
			http.NotFound(w, r)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		checkBytes, err := json.Marshal(check)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if !abilities.CanRead(check) {
			authorization.UnauthorizedAccessToResource(w)
			return
		}

		fmt.Fprintf(w, string(checkBytes))
	case http.MethodPut, http.MethodPost:
		newCheck := &types.CheckConfig{}
		bodyBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		err = json.Unmarshal(bodyBytes, newCheck)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err = newCheck.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch {
		case check == nil && !abilities.CanCreate(newCheck):
			fallthrough
		case check != nil && !abilities.CanUpdate(newCheck):
			authorization.UnauthorizedAccessToResource(w)
			return
		}

		err = c.Store.UpdateCheckConfig(r.Context(), newCheck)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	case http.MethodDelete:
		err := c.Store.DeleteCheckConfigByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}
}

func rejectChecks(records *[]*types.CheckConfig, predicate func(*types.CheckConfig) bool) {
	for i := 0; i < len(*records); i++ {
		if !predicate((*records)[i]) {
			*records = append((*records)[:i], (*records)[i+1:]...)
			i--
		}
	}
}
