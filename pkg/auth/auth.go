package auth

import (
	"fmt"
	"github.com/complyue/ddgo/pkg/dbc"
	"github.com/complyue/hbigo/pkg/errors"
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/golang/glog"
	"thingswell.com/ids-base/server/routers"
)

func coll() *mgo.Collection {
	return dbc.DB().C("atkn")
}

func tokenExpired(tid, uid, tkn string) bool {
	// TODO implement expiration
	return false
}

// proc local cache
var validatedTokens = make(map[string]struct{})

func validateToken(tid, uid, tkn string) (bool, error) {
	fqk := fmt.Sprintf("%s/%s/%s", tid, uid, tkn)
	qc := bson.M{
		"tid": tid, "uid": uid, "tkn": tkn,
	}

	// consult local cache first
	if _, ok := validatedTokens[fqk]; ok {
		// cache hit

		if !tokenExpired(tid, uid, tkn) {
			return true, nil
		}

		// remove from cache & db
		delete(validatedTokens, fqk)
		coll().Remove(qc)
		return false, nil
	}
	// cache miss
	// contact db
	if n, err := coll().Find(qc).Limit(1).Count(); err != nil {
		return false, err
	} else if n > 0 {
		// db has record
		if !tokenExpired(tid, uid, tkn) {
			// add to cache
			validatedTokens[fqk] = struct{}{}
			return true, nil
		}

		// remove from db
		coll().Remove(qc)
	}
	// no valid record
	return false, nil
}

func AuthenticateUser(tid, uid, pwd, tkn string) (string, error) {
	if tkn != "" {
		if ok, err := validateToken(tid, uid, tkn); err != nil {
			return "", err
		} else if ok {
			return tkn, nil
		}
		glog.Warningf("Invalid token auth attempted. %s/%s[%s]\n", tid, uid, tkn)
		// todo threat detection & mitigation
	}

	if pwd == "" {
		return "", nil
	}

	resp, err := accountLogin(routers.LoginReq{
		MobileNo: uid, Password: pwd, VCode: "",
	})
	if err != nil {
		return "", nil
	}
	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}

	// generate auth token
	tkn = bson.NewObjectId().Hex()
	rec := bson.M{
		"_id": bson.NewObjectId(),
		"tid": tid, "uid": uid, "tkn": tkn,
	}
	// insert into db
	if err := coll().Insert(rec); err != nil {
		glog.Errorf("Error inserting auth token: %s\n", err)
		return "", err
	}
	fqk := fmt.Sprintf("%s/%s/%s", tid, uid, tkn)
	// put into cache
	validatedTokens[fqk] = struct{}{}

	return tkn, nil
}

func RegisterUser(tid, uid, pwd string) (bool, error) {
	if pwd == "" {
		return false, errors.New("Password can't be empty!")
	}

	resp, err := accountCreate(routers.AccountCreateReq{
		Name: uid, MobileNo: uid, Password: pwd,
		TIDs: []string{tid},
	})
	if err != nil {
		return false, err
	}
	if resp.Error != "" {
		return resp.Result, errors.New(resp.Error)
	}
	return resp.Result, nil
}
