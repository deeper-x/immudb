/*
Copyright 2019-2020 vChain, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codenotary/immudb/pkg/api/schema"
	"github.com/codenotary/immudb/pkg/auth"
	"github.com/codenotary/immudb/pkg/immuos"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

var testDatabase = "lisbon"
var testUsername = []byte("Sagrada")
var testPassword = []byte("Familia@2")
var testKey = []byte("Antoni")
var testValue = []byte("Gaudí")

func newInmemoryAuthServer() *ImmuServer {
	dbRootpath := DefaultOption().GetDbRootPath()
	s := DefaultServer()
	s = s.WithOptions(s.Options.WithAuth(true).WithInMemoryStore(true))
	err := s.loadDefaultDatabase(dbRootpath)
	if err != nil {
		log.Fatal(err)
	}
	err = s.loadSystemDatabase(dbRootpath)
	if err != nil {
		log.Fatal(err)
	}
	return s
}

func newAuthServer(dbroot string) *ImmuServer {
	dbRootpath := DefaultOption().WithDbRootPath(dbroot).GetDbRootPath()
	s := DefaultServer()
	s = s.WithOptions(s.Options.WithAuth(true).WithDir(dbRootpath).WithCorruptionCheck(false))
	err := s.loadDefaultDatabase(dbRootpath)
	if err != nil {
		log.Fatal(err)
	}
	err = s.loadSystemDatabase(dbRootpath)
	if err != nil {
		log.Fatal(err)
	}
	err = s.loadUserDatabases(dbRootpath)
	if err != nil {
		log.Fatal(err)
	}
	return s
}
func login(s *ImmuServer, username, password string) (context.Context, error) {
	r := &schema.LoginRequest{
		User:     []byte(username),
		Password: []byte(password),
	}
	ctx := context.Background()
	l, err := s.Login(ctx, r)
	if err != nil {
		log.Fatal(err)
	}
	m := make(map[string]string)
	m["Authorization"] = "Bearer " + string(l.Token)
	ctx = metadata.NewIncomingContext(ctx, metadata.New(m))
	return ctx, nil
}

func usedatabase(ctx context.Context, s *ImmuServer, dbname string) (context.Context, error) {
	token, err := s.UseDatabase(ctx, &schema.Database{
		Databasename: dbname,
	})
	if err != nil {
		return nil, err
	}

	m := make(map[string]string)
	m["Authorization"] = "Bearer " + string(token.Token)
	ctx = metadata.NewIncomingContext(ctx, metadata.New(m))

	return ctx, nil
}

func TestServerDefaultDatabaseLoad(t *testing.T) {
	options := DefaultOption()
	dbRootpath := options.GetDbRootPath()
	s := DefaultServer()
	err := s.loadDefaultDatabase(dbRootpath)
	if err != nil {
		t.Fatalf("error loading default database %v", err)
	}
	defer func() {
		os.RemoveAll(dbRootpath)
	}()
	_, err = os.Stat(path.Join(options.GetDbRootPath(), DefaultOptions().defaultDbName))
	if os.IsNotExist(err) {
		t.Fatalf("default database directory not created")
	}
}
func TestServerSystemDatabaseLoad(t *testing.T) {
	serverOptions := DefaultOptions().WithDir("Nice")
	options := DefaultOption().WithDbRootPath(serverOptions.Dir)
	dbRootpath := options.GetDbRootPath()
	s := DefaultServer().WithOptions(serverOptions)
	err := s.loadDefaultDatabase(dbRootpath)
	if err != nil {
		t.Fatalf("error loading default database %v", err)
	}
	err = s.loadSystemDatabase(dbRootpath)
	if err != nil {
		t.Fatalf("error loading system database %v", err)
	}
	defer func() {
		os.RemoveAll(dbRootpath)
	}()
	_, err = os.Stat(path.Join(options.GetDbRootPath(), DefaultOptions().GetSystemAdminDbName()))
	if os.IsNotExist(err) {
		t.Fatalf("system database directory not created")
	}
}
func TestServerLogin(t *testing.T) {
	s := newInmemoryAuthServer()
	r := &schema.LoginRequest{
		User:     []byte(auth.SysAdminUsername),
		Password: []byte(auth.SysAdminPassword),
	}
	resp, err := s.Login(context.Background(), r)
	if err != nil {
		t.Fatalf("Login error %v", err)
	}
	if len(resp.Token) == 0 {
		t.Fatalf("login token is empty")
	}
	if len(resp.Warning) == 0 {
		t.Fatalf("default immudb password missing warning")
	}
}
func TestServerLogout(t *testing.T) {
	s := newInmemoryAuthServer()
	_, err := s.Logout(context.Background(), &emptypb.Empty{})
	if err == nil || err.Error() != "rpc error: code = Internal desc = no headers found on request" {
		t.Fatalf("Logout expected error, got %v", err)
	}

	r := &schema.LoginRequest{
		User:     []byte(auth.SysAdminUsername),
		Password: []byte(auth.SysAdminPassword),
	}
	ctx := context.Background()
	l, err := s.Login(ctx, r)
	if err != nil {
		t.Fatalf("Login error %v", err)
	}
	m := make(map[string]string)
	m["Authorization"] = "Bearer " + string(l.Token)
	ctx = metadata.NewIncomingContext(ctx, metadata.New(m))
	_, err = s.Logout(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Logout error %v", err)
	}
}
func TestServerCreateDatabase(t *testing.T) {
	s := newInmemoryAuthServer()
	ctx, err := login(s, auth.SysAdminUsername, auth.SysAdminPassword)
	if err != nil {
		t.Fatalf("Login error %v", err)
	}
	newdb := &schema.Database{
		Databasename: "lisbon",
	}
	dbrepl, err := s.CreateDatabase(ctx, newdb)
	if err != nil {
		t.Fatalf("Createdatabase error %v", err)
	}
	if dbrepl.Error.Errorcode != schema.ErrorCodes_Ok {
		t.Fatalf("Createdatabase error %v", dbrepl)
	}
}
func TestServerLoaduserDatabase(t *testing.T) {
	s := newAuthServer("loaduserdatabase")
	ctx, err := login(s, auth.SysAdminUsername, auth.SysAdminPassword)
	if err != nil {
		t.Fatalf("Login error %v", err)
	}
	defer os.RemoveAll(s.Options.Dir)
	newdb := &schema.Database{
		Databasename: testDatabase,
	}
	dbrepl, err := s.CreateDatabase(ctx, newdb)
	if err != nil {
		t.Fatalf("Createdatabase error %v", err)
	}
	if dbrepl.Error.Errorcode != schema.ErrorCodes_Ok {
		t.Fatalf("Createdatabase error %v", dbrepl)
	}
	err = s.CloseDatabases()
	if err != nil {
		t.Fatalf("closedatabases error %v", err)
	}
	time.Sleep(1 * time.Second)
	s = newAuthServer("loaduserdatabase")
	if s.dbList.Length() != 2 {
		t.Fatalf("LoadUserDatabase error %d", s.dbList.Length())
	}

	// Walk error
	errWalk := errors.New("Walk error")
	s.OS.(*immuos.StandardOS).WalkF = func(root string, walkFn filepath.WalkFunc) error {
		return walkFn("", nil, errWalk)
	}
	require.Equal(t, errWalk, s.loadUserDatabases("loaduserdatabase"))
}
func testServerCreateUser(ctx context.Context, s *ImmuServer, t *testing.T) {
	newUser := &schema.CreateUserRequest{
		User:       testUsername,
		Password:   testPassword,
		Database:   testDatabase,
		Permission: auth.PermissionAdmin,
	}
	userresp, err := s.CreateUser(ctx, newUser)
	if err != nil {
		t.Fatalf("CreateUser error %v", err)
	}
	if !bytes.Equal(userresp.User, testUsername) {
		t.Fatalf("CreateUser error username does not match %v", userresp)
	}
	if userresp.Permission != auth.PermissionAdmin {
		t.Fatalf("CreateUser error permission does not match %v", userresp)
	}

	if !s.mandatoryAuth() {
		t.Fatalf("mandatoryAuth expected true")
	}
}
func testServerListUsers(ctx context.Context, s *ImmuServer, t *testing.T) {
	users, err := s.ListUsers(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListUsers error %v", err)
	}
	if len(users.Users) < 1 {
		t.Fatalf("List users, expected >1 got %v", len(users.Users))
	}
}

func TestServerListUsersAdmin(t *testing.T) {
	srv := newAuthServer("listusersadmin")
	ctx, err := login(srv, auth.SysAdminUsername, auth.SysAdminPassword)
	if err != nil {
		t.Fatalf("login error %v", err)
	}
	defer os.RemoveAll("listusersadmin")
	newdb := &schema.Database{
		Databasename: testDatabase,
	}
	_, err = srv.CreateDatabase(ctx, newdb)
	if err != nil {
		t.Fatal(err)
	}
	newUser := &schema.CreateUserRequest{
		User:       testUsername,
		Password:   testPassword,
		Database:   testDatabase,
		Permission: auth.PermissionAdmin,
	}
	_, err = srv.CreateUser(ctx, newUser)
	if err != nil {
		t.Fatalf("CreateUser error %v", err)
	}
	srv.multidbmode = true
	ctx, err = login(srv, string(testUsername), string(testPassword))
	if err != nil {
		t.Fatalf("login error %v", err)
	}
	ctx, err = usedatabase(ctx, srv, testDatabase)
	if err != nil {
		t.Fatalf("UseDatabase error %v", err)
	}
	users, err := srv.ListUsers(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListUsers error %v", err)
	}
	if len(users.Users) < 1 {
		t.Fatalf("List users, expected >1 got %v", len(users.Users))
	}

	ctx, err = login(srv, auth.SysAdminUsername, auth.SysAdminPassword)
	if err != nil {
		t.Fatalf("login error %v", err)
	}
	users, err = srv.ListUsers(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListUsers error %v", err)
	}
	if len(users.Users) < 1 {
		t.Fatalf("List users, expected >1 got %v", len(users.Users))
	}

	newUser = &schema.CreateUserRequest{
		User:       []byte("rwuser"),
		Password:   []byte("rwuserPas@1"),
		Database:   testDatabase,
		Permission: auth.PermissionRW,
	}
	_, err = srv.CreateUser(ctx, newUser)
	if err != nil {
		t.Fatalf("CreateUser error %v", err)
	}
	srv.multidbmode = true
	ctx, err = login(srv, "rwuser", "rwuserPas@1")
	if err != nil {
		t.Fatalf("login error %v", err)
	}
	ctx, err = usedatabase(ctx, srv, testDatabase)
	if err != nil {
		t.Fatalf("UseDatabase error %v", err)
	}
	users, err = srv.ListUsers(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("ListUsers error %v", err)
	}
	if len(users.Users) < 1 {
		t.Fatalf("List users, expected >1 got %v", len(users.Users))
	}
}

func testServerListDatabases(ctx context.Context, s *ImmuServer, t *testing.T) {
	dbs, err := s.DatabaseList(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("DatabaseList error %v", err)
	}
	if len(dbs.Databases) < 1 {
		t.Fatalf("List databases, expected >1 got %v", len(dbs.Databases))
	}
}
func testServerUseDatabase(ctx context.Context, s *ImmuServer, t *testing.T) {
	dbs, err := s.UseDatabase(ctx, &schema.Database{
		Databasename: testDatabase,
	})
	if err != nil {
		t.Fatalf("UseDatabase error %v", err)
	}
	if dbs.Error.Errorcode != schema.ErrorCodes_Ok {
		t.Fatalf("Error selecting database %v", dbs)
	}
	if len(dbs.Token) == 0 {
		t.Fatalf("Expected token, got %v", dbs.Token)
	}
	m := make(map[string]string)
	m["Authorization"] = "Bearer " + string(dbs.Token)
	ctx = metadata.NewIncomingContext(ctx, metadata.New(m))
}
func testServerChangePermission(ctx context.Context, s *ImmuServer, t *testing.T) {
	perm, err := s.ChangePermission(ctx, &schema.ChangePermissionRequest{
		Action:     schema.PermissionAction_GRANT,
		Database:   testDatabase,
		Permission: auth.PermissionR,
		Username:   string(testUsername),
	})
	if err != nil {
		t.Fatalf("DatabaseList error %v", err)
	}
	if perm.Errorcode != schema.ErrorCodes_Ok {
		t.Fatalf("error changing permission, got %v", perm)
	}
}
func testServerDeactivateUser(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.SetActiveUser(ctx, &schema.SetActiveUserRequest{
		Active:   false,
		Username: string(testUsername),
	})
	if err != nil {
		t.Fatalf("DeactivateUser error %v", err)
	}
}
func testServerSetActiveUser(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.SetActiveUser(ctx, &schema.SetActiveUserRequest{
		Active:   true,
		Username: string(testUsername),
	})
	if err != nil {
		t.Fatalf("SetActiveUser error %v", err)
	}
}
func testServerChangePassword(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.ChangePassword(ctx, &schema.ChangePasswordRequest{
		NewPassword: testPassword,
		OldPassword: testPassword,
		User:        testUsername,
	})
	if err != nil {
		t.Fatalf("ChangePassword error %v", err)
	}
}

func testServerSetGet(ctx context.Context, s *ImmuServer, t *testing.T) {
	ind, err := s.Set(ctx, &schema.KeyValue{
		Key:   testKey,
		Value: testValue,
	})
	if err != nil {
		t.Fatalf("set error %v", err)
	}

	it, err := s.Get(ctx, &schema.Key{
		Key: testKey,
	})
	if err != nil {
		t.Fatalf("Get error %v", err)
	}
	if it.Index != ind.Index {
		t.Fatalf("set.get index missmatch expected %v got %v", ind, it.Index)
	}
	if !bytes.Equal(it.Key, testKey) {
		t.Fatalf("get key missmatch expected %v got %v", string(testKey), string(it.Key))
	}
	if !bytes.Equal(it.Value, testValue) {
		t.Fatalf("get key missmatch expected %v got %v", string(testValue), string(it.Value))
	}
}

func testServerSetGetError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Set(context.Background(), &schema.KeyValue{
		Key:   testKey,
		Value: testValue,
	})
	if err == nil {
		t.Fatalf("set expected error")
	}

	_, err = s.Get(context.Background(), &schema.Key{
		Key: testKey,
	})
	if err == nil {
		t.Fatalf("get expected error")
	}
}

func testServerSafeSetGet(ctx context.Context, s *ImmuServer, t *testing.T) {
	root, err := s.CurrentRoot(ctx, &emptypb.Empty{})
	if err != nil {
		t.Error(err)
	}
	kv := []*schema.SafeSetOptions{
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Alberto"),
				Value: []byte("Tomba"),
			},
			RootIndex: &schema.Index{
				Index: root.Index,
			},
		},
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Jean-Claude"),
				Value: []byte("Killy"),
			},
			RootIndex: &schema.Index{
				Index: root.Index,
			},
		},
		{
			Kv: &schema.KeyValue{
				Key:   []byte("Franz"),
				Value: []byte("Clamer"),
			},
			RootIndex: &schema.Index{
				Index: root.Index,
			},
		},
	}
	for _, val := range kv {
		proof, err := s.SafeSet(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
		if proof == nil {
			t.Fatalf("Nil proof after SafeSet")
		}
		it, err := s.SafeGet(ctx, &schema.SafeGetOptions{
			Key: val.Kv.Key,
		})
		if it.GetItem().GetIndex() != proof.Index {
			t.Fatalf("SafeGet index error, expected %d, got %d", proof.Index, it.GetItem().GetIndex())
		}
	}
}

func testServerCurrentRoot(ctx context.Context, s *ImmuServer, t *testing.T) {
	for _, val := range kv {
		_, err := s.Set(ctx, val)
		if err != nil {
			t.Fatalf("CurrentRoot Error Inserting to db %s", err)
		}
		_, err = s.CurrentRoot(ctx, &emptypb.Empty{})
		if err != nil {
			t.Fatalf("CurrentRoot Error getting current root %s", err)
		}
	}
}
func testServerCurrentRootError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.CurrentRoot(context.Background(), &emptypb.Empty{})
	if err == nil {
		t.Fatalf("CurrentRoot expected Error")
	}
}
func testServerSVSetGet(ctx context.Context, s *ImmuServer, t *testing.T) {
	for _, val := range Skv.SKVs {
		it, err := s.SetSV(ctx, val)
		if err != nil {
			t.Fatalf("SetSV Error Inserting to db %s", err)
		}
		k := &schema.Key{
			Key: []byte(val.Key),
		}
		item, err := s.GetSV(ctx, k)
		if err != nil {
			t.Fatalf("GetSV Error reading key %s", err)
		}

		if it.GetIndex() != item.Index {
			t.Fatalf("index error expecting %v got %v", item.Index, it.GetIndex())
		}
		if !bytes.Equal(item.GetKey(), val.Key) {
			t.Fatalf("Inserted Key not equal to read Key")
		}
		sk := item.GetValue()
		if sk.GetTimestamp() != val.GetValue().GetTimestamp() {
			t.Fatalf("Inserted value not equal to read value")
		}
		if !bytes.Equal(sk.GetPayload(), val.GetValue().Payload) {
			t.Fatalf("Inserted Payload not equal to read value")
		}
	}
}

func testServerSVSetGetError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.SetSV(context.Background(), Skv.SKVs[0])
	if err == nil {
		t.Fatalf("SetSV expected error Inserting to db")
	}
	k := &schema.Key{
		Key: []byte(Skv.SKVs[0].Key),
	}
	_, err = s.GetSV(context.Background(), k)
	if err == nil {
		t.Fatalf("GetSV error reading key")
	}
}

func testServerSafeSetGetSV(ctx context.Context, s *ImmuServer, t *testing.T) {
	root, err := s.CurrentRoot(ctx, &emptypb.Empty{})
	if err != nil {
		t.Error(err)
	}
	SafeSkv := []*schema.SafeSetSVOptions{
		{
			Skv: &schema.StructuredKeyValue{
				Key: []byte("Alberto"),
				Value: &schema.Content{
					Timestamp: uint64(time.Now().Unix()),
					Payload:   []byte("Tomba"),
				},
			},
			RootIndex: &schema.Index{
				Index: root.Index,
			},
		},
		{
			Skv: &schema.StructuredKeyValue{
				Key: []byte("Jean-Claude"),
				Value: &schema.Content{
					Timestamp: uint64(time.Now().Unix()),
					Payload:   []byte("Killy"),
				},
			},
			RootIndex: &schema.Index{
				Index: root.Index,
			},
		},
		{
			Skv: &schema.StructuredKeyValue{
				Key: []byte("Franz"),
				Value: &schema.Content{
					Timestamp: uint64(time.Now().Unix()),
					Payload:   []byte("Clamer"),
				},
			},
			RootIndex: &schema.Index{
				Index: root.Index,
			},
		},
	}
	for _, val := range SafeSkv {
		proof, err := s.SafeSetSV(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
		if proof == nil {
			t.Fatalf("Nil proof after SafeSet")
		}
		it, err := s.SafeGetSV(ctx, &schema.SafeGetOptions{
			Key: val.Skv.Key,
		})
		if it.GetItem().GetIndex() != proof.Index {
			t.Fatalf("SafeGet index error, expected %d, got %d", proof.Index, it.GetItem().GetIndex())
		}
	}
}
func testServerSafeSetGetSVError(ctx context.Context, s *ImmuServer, t *testing.T) {
	Skv := schema.SafeSetSVOptions{
		Skv: &schema.StructuredKeyValue{
			Key: []byte("Alberto"),
			Value: &schema.Content{
				Timestamp: uint64(time.Now().Unix()),
				Payload:   []byte("Tomba"),
			},
		},
		RootIndex: &schema.Index{
			Index: 0,
		}}
	_, err := s.SafeSetSV(context.Background(), &Skv)
	if err == nil {
		t.Fatalf("SafeSetSV expected error")
	}
	_, err = s.SafeGetSV(context.Background(), &schema.SafeGetOptions{
		Key: Skv.Skv.Key,
	})
	if err == nil {
		t.Fatalf("SafeGetSV expected error")
	}
}
func testServerSetGetBatch(ctx context.Context, s *ImmuServer, t *testing.T) {
	Skv := &schema.KVList{
		KVs: []*schema.KeyValue{
			{
				Key:   []byte("Alberto"),
				Value: []byte("Tomba"),
			},
			{
				Key:   []byte("Jean-Claude"),
				Value: []byte("Killy"),
			},
			{
				Key:   []byte("Franz"),
				Value: []byte("Clamer"),
			},
		},
	}
	ind, err := s.SetBatch(ctx, Skv)
	if err != nil {
		t.Fatalf("Error Inserting to db %s", err)
	}
	if ind == nil {
		t.Fatalf("Nil index after Setbatch")
	}

	itList, err := s.GetBatch(ctx, &schema.KeyList{
		Keys: []*schema.Key{
			{
				Key: []byte("Alberto"),
			},
			{
				Key: []byte("Jean-Claude"),
			},
			{
				Key: []byte("Franz"),
			},
		},
	})
	for ind, val := range itList.Items {
		if !bytes.Equal(val.Value, Skv.KVs[ind].Value) {
			t.Fatalf("BatchSet value not equal to BatchGet value, expected %s, got %s", string(Skv.KVs[ind].Value), string(val.Value))
		}
	}
}

func testServerSetGetBatchError(ctx context.Context, s *ImmuServer, t *testing.T) {
	Skv := &schema.KVList{
		KVs: []*schema.KeyValue{
			{
				Key:   []byte("Alberto"),
				Value: []byte("Tomba"),
			},
		},
	}
	_, err := s.SetBatch(context.Background(), Skv)
	if err == nil {
		t.Fatalf("SetBatch expected Error")
	}
	_, err = s.GetBatch(context.Background(), &schema.KeyList{
		Keys: []*schema.Key{
			{
				Key: []byte("Alberto"),
			},
		},
	})
	if err == nil {
		t.Fatalf("GetBatch expected Error")
	}
}

func testServerSetGetBatchSV(ctx context.Context, s *ImmuServer, t *testing.T) {
	ind, err := s.SetBatchSV(ctx, Skv)
	if err != nil {
		t.Fatalf("Error Inserting to db %s", err)
	}
	if ind == nil {
		t.Fatalf("Nil index after Setbatch")
	}
	itList, err := s.GetBatchSV(ctx, &schema.KeyList{
		Keys: []*schema.Key{
			{
				Key: Skv.SKVs[0].Key,
			},
			{
				Key: Skv.SKVs[1].Key,
			},
			{
				Key: Skv.SKVs[2].Key,
			},
		},
	})
	for ind, val := range itList.Items {
		if !bytes.Equal(val.Value.Payload, Skv.SKVs[ind].Value.Payload) {
			t.Fatalf("BatchSetSV value not equal to BatchGetSV value, expected %s, got %s", string(Skv.SKVs[ind].Value.Payload), string(val.Value.Payload))
		}
		if val.Value.Timestamp != Skv.SKVs[ind].Value.Timestamp {
			t.Fatalf("BatchSetSV value not equal to BatchGetSV value, expected %d, got %d", Skv.SKVs[ind].Value.Timestamp, val.Value.Timestamp)
		}
	}
}

func testServerSetGetBatchSVError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.SetBatchSV(context.Background(), Skv)
	if err == nil {
		t.Fatalf("SetBatchSV expected error")
	}
	_, err = s.GetBatchSV(context.Background(), &schema.KeyList{
		Keys: []*schema.Key{
			{
				Key: Skv.SKVs[0].Key,
			},
		},
	})
	if err == nil {
		t.Fatalf("GetBatchSV expected error")
	}
}

func testServerInclusion(ctx context.Context, s *ImmuServer, t *testing.T) {
	for _, val := range kv {
		_, err := s.Set(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
	}
	ind := uint64(1)
	inc, err := s.Inclusion(ctx, &schema.Index{Index: ind})
	if err != nil {
		t.Fatalf("Inclusion Error to db %s", err)
	}
	if inc.Index != ind {
		t.Fatalf("Inclusion, expected %d, got %d", inc.Index, ind)
	}
}

func testServerInclusionError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Inclusion(context.Background(), &schema.Index{Index: 0})
	if err == nil {
		t.Fatalf("Inclusion expected error")
	}
}

func testServerConsintency(ctx context.Context, s *ImmuServer, t *testing.T) {
	for _, val := range kv {
		_, err := s.Set(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
	}
	ind := uint64(1)
	inc, err := s.Consistency(ctx, &schema.Index{Index: ind})
	if err != nil {
		t.Fatalf("Consistency Error %s", err)
	}
	if inc.First != ind {
		t.Fatalf("Consistency, expected %d, got %d", inc.First, ind)
	}
}
func testServerConsintencyError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Consistency(context.Background(), &schema.Index{Index: 0})
	if err == nil {
		t.Fatalf("Consistency expected error")
	}
}
func testServerByIndex(ctx context.Context, s *ImmuServer, t *testing.T) {

	ind := uint64(1)
	for _, val := range kv {
		it, err := s.Set(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
		ind = it.Index
	}
	s.SafeSet(ctx, &schema.SafeSetOptions{
		Kv: &schema.KeyValue{
			Key:   testKey,
			Value: testValue,
		},
	})
	inc, err := s.ByIndex(ctx, &schema.Index{Index: ind})
	if err != nil {
		t.Fatalf("ByIndex Error %s", err)
	}
	if !bytes.Equal(inc.Value, kv[len(kv)-1].Value) {
		t.Fatalf("ByIndex, expected %s, got %d", kv[ind].Value, inc.Value)
	}
}
func testServerByIndexError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.ByIndex(context.Background(), &schema.Index{Index: 0})
	if err == nil {
		t.Fatal("ByIndex exptected error")
	}
}
func testServerByIndexSV(ctx context.Context, s *ImmuServer, t *testing.T) {
	ind := uint64(2)
	for _, val := range Skv.SKVs {
		it, err := s.SetSV(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
		ind = it.Index
	}
	s.SafeSet(ctx, &schema.SafeSetOptions{
		Kv: &schema.KeyValue{
			Key:   testKey,
			Value: testValue,
		},
	})
	inc, err := s.ByIndexSV(ctx, &schema.Index{Index: ind})
	if err != nil {
		t.Fatalf("ByIndexSV Error %s", err)
	}
	if !bytes.Equal(inc.Value.Payload, Skv.SKVs[len(Skv.SKVs)-1].Value.Payload) {
		t.Fatalf("ByIndexSV, expected %s, got %s", string(Skv.SKVs[len(Skv.SKVs)-1].Value.Payload), string(inc.Value.Payload))
	}
}

func testServerByIndexSVError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.ByIndexSV(context.Background(), &schema.Index{Index: 0})
	if err == nil {
		t.Fatalf("ByIndexSV exptected error")
	}
}

func testServerByIScanSV(ctx context.Context, s *ImmuServer, t *testing.T) {
	for _, val := range Skv.SKVs {
		_, err := s.SetSV(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
	}
	s.SafeSet(ctx, &schema.SafeSetOptions{
		Kv: &schema.KeyValue{
			Key:   testKey,
			Value: testValue,
		},
	})

	_, err := s.IScanSV(ctx, &schema.IScanOptions{
		PageNumber: 1,
		PageSize:   1,
	})
	assert.Errorf(t, err, schema.ErrUnexpectedNotStructuredValue.Error())
}
func testServerByIScanSVError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.IScanSV(context.Background(), &schema.IScanOptions{
		PageNumber: 1,
		PageSize:   1,
	})
	assert.Error(t, err)
}

func testServerBySafeIndex(ctx context.Context, s *ImmuServer, t *testing.T) {
	for _, val := range Skv.SKVs {
		_, err := s.SetSV(ctx, val)
		if err != nil {
			t.Fatalf("Error Inserting to db %s", err)
		}
	}
	s.SafeSet(ctx, &schema.SafeSetOptions{
		Kv: &schema.KeyValue{
			Key:   testKey,
			Value: testValue,
		},
	})
	ind := uint64(1)
	inc, err := s.BySafeIndex(ctx, &schema.SafeIndexOptions{Index: ind})
	if err != nil {
		t.Fatalf("Error Inserting to db %s", err)
	}
	if inc.Item.Index != ind {
		t.Fatalf("ByIndexSV, expected %d, got %d", ind, inc.Item.Index)
	}
}

func testServerBySafeIndexError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.BySafeIndex(context.Background(), &schema.SafeIndexOptions{Index: 0})
	if err == nil {
		t.Fatalf("BySafeIndex exptected error")
	}
}

func testServerHistory(ctx context.Context, s *ImmuServer, t *testing.T) {
	inc, err := s.History(ctx, &schema.Key{
		Key: testKey,
	})
	if err != nil {
		t.Fatalf("History Error %s", err)
	}
	for _, val := range inc.Items {
		if !bytes.Equal(val.Value, testValue) {
			t.Fatalf("History, expected %s, got %s", val.Value, testValue)
		}
	}
}

func testServerHistoryError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.History(context.Background(), &schema.Key{
		Key: testKey,
	})
	if err == nil {
		t.Fatalf("History exptected error")
	}
}

func testServerHistorySV(ctx context.Context, s *ImmuServer, t *testing.T) {
	k := &schema.Key{
		Key: testValue,
	}
	items, err := s.HistorySV(ctx, k)
	if err != nil {
		t.Fatalf("Error reading key %s", err)
	}
	for _, val := range items.Items {
		if !bytes.Equal(val.Value.Payload, testValue) {
			t.Fatalf("HistorySV, expected %s, got %s", testValue, val.Value.Payload)
		}
	}
}

func testServerHistorySVError(ctx context.Context, s *ImmuServer, t *testing.T) {
	k := &schema.Key{
		Key: testValue,
	}
	_, err := s.HistorySV(context.Background(), k)
	if err == nil {
		t.Fatalf("HistorySV ecptected Error")
	}
}

func testServerHealth(ctx context.Context, s *ImmuServer, t *testing.T) {
	h, err := s.Health(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("health error %s", err)
	}
	if !h.GetStatus() {
		t.Fatalf("Health, expected %v, got %v", true, h.GetStatus())
	}
}
func testServerHealthError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Health(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("health exptected error")
	}
}

func testServerReference(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Set(ctx, kv[0])
	if err != nil {
		t.Fatalf("Reference error %s", err)
	}
	_, err = s.Reference(ctx, &schema.ReferenceOptions{
		Reference: []byte(`tag`),
		Key:       kv[0].Key,
	})
	item, err := s.Get(ctx, &schema.Key{Key: []byte(`tag`)})
	if err != nil {
		t.Fatalf("Reference  Get error %s", err)
	}
	if !bytes.Equal(item.Value, kv[0].Value) {
		t.Fatalf("Reference, expected %v, got %v", string(item.Value), string(kv[0].Value))
	}
}

func testServerReferenceError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Reference(ctx, &schema.ReferenceOptions{
		Reference: []byte(`tag`),
		Key:       kv[0].Key,
	})
	if err != nil {
		t.Fatalf("Reference  exptected  error")
	}
}

func testServerZAdd(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Set(ctx, kv[0])
	if err != nil {
		t.Fatalf("Reference error %s", err)
	}
	_, err = s.ZAdd(ctx, &schema.ZAddOptions{
		Key:   kv[0].Key,
		Score: 1,
		Set:   kv[0].Value,
	})
	item, err := s.ZScan(ctx, &schema.ZScanOptions{
		Offset:  []byte(""),
		Limit:   3,
		Reverse: false,
	})
	if err != nil {
		t.Fatalf("Reference  Get error %s", err)
	}
	if !bytes.Equal(item.Items[0].Value, kv[0].Value) {
		t.Fatalf("Reference, expected %v, got %v", string(kv[0].Value), string(item.Items[0].Value))
	}
}
func testServerZAddError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.ZAdd(context.Background(), &schema.ZAddOptions{
		Key:   kv[0].Key,
		Score: 1,
		Set:   kv[0].Value,
	})
	if err == nil {
		t.Fatalf("ZAdd expected errr")
	}
	_, err = s.ZScan(context.Background(), &schema.ZScanOptions{
		Offset:  []byte(""),
		Limit:   3,
		Reverse: false,
	})
	if err == nil {
		t.Fatalf("ZScan expected errr")
	}
}

func testServerScan(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Set(ctx, kv[0])
	if err != nil {
		t.Fatalf("set error %s", err)
	}
	_, err = s.ZAdd(ctx, &schema.ZAddOptions{
		Key:   kv[0].Key,
		Score: 3,
		Set:   kv[0].Value,
	})
	if err != nil {
		t.Fatalf("zadd error %s", err)
	}

	_, err = s.SafeZAdd(ctx, &schema.SafeZAddOptions{
		Zopts: &schema.ZAddOptions{
			Key:   kv[0].Key,
			Score: 0,
			Set:   kv[0].Value,
		},
		RootIndex: &schema.Index{
			Index: 0,
		},
	})
	if err != nil {
		t.Fatalf("SafeZAdd error %s", err)
	}

	item, err := s.Scan(ctx, &schema.ScanOptions{
		Offset: nil,
		Deep:   false,
		Limit:  1,
		Prefix: kv[0].Key,
	})

	if err != nil {
		t.Fatalf("Scan  Get error %s", err)
	}
	if !bytes.Equal(item.Items[0].Key, kv[0].Key) {
		t.Fatalf("Reference, expected %v, got %v", string(kv[0].Key), string(item.Items[0].Key))
	}

	scanItem, err := s.IScan(ctx, &schema.IScanOptions{
		PageNumber: 2,
		PageSize:   1,
	})
	if err != nil {
		t.Fatalf("IScan  Get error %s", err)
	}
	if !bytes.Equal(scanItem.Items[0].Key, kv[0].Key) {
		t.Fatalf("Reference, expected %v, got %v", string(kv[0].Key), string(scanItem.Items[0].Value))
	}
}

func testServerScanError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Scan(context.Background(), &schema.ScanOptions{
		Offset: nil,
		Deep:   false,
		Limit:  1,
		Prefix: kv[0].Key,
	})
	if err == nil {
		t.Fatalf("Scan exptected error")
	}
	_, err = s.IScan(context.Background(), &schema.IScanOptions{
		PageNumber: 2,
		PageSize:   1,
	})
	if err == nil {
		t.Fatalf("IScan  exptected error")
	}
}

func testServerScanSV(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.SafeSetSV(ctx, &schema.SafeSetSVOptions{
		Skv: Skv.SKVs[0],
	})
	if err != nil {
		t.Fatalf("set error %s", err)
	}

	scanItem, err := s.ZScanSV(ctx, &schema.ZScanOptions{
		Offset: []byte("Albert"),
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("ZScanSV  Get error %s", err)
	}
	if len(scanItem.Items) == 0 {
		t.Fatalf("ZScanSV, expected >0, got %v", len(scanItem.Items))
	}

	scanItem, err = s.ScanSV(ctx, &schema.ScanOptions{
		Offset: []byte("Alb"),
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("ScanSV  Get error %s", err)
	}
	if len(scanItem.Items) == 0 {
		t.Fatalf("ScanSV, expected >0, got %v", len(scanItem.Items))
	}
}

func testServerScanSVError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.ScanSV(context.Background(), &schema.ScanOptions{
		Offset: []byte("Alb"),
		Limit:  1,
	})
	if err == nil {
		t.Fatalf("ScanSV  exptected error")
	}
}

func testServerSafeReference(ctx context.Context, s *ImmuServer, t *testing.T) {
	it, err := s.SafeSet(ctx, &schema.SafeSetOptions{
		Kv: kv[0],
	})
	if err != nil {
		t.Fatalf("SafeSet error %s", err)
	}
	it, err = s.SafeReference(ctx, &schema.SafeReferenceOptions{
		Ro: &schema.ReferenceOptions{
			Key:       kv[0].Key,
			Reference: []byte("key1"),
		},
		RootIndex: &schema.Index{
			Index: it.Index,
		},
	})
	if err != nil {
		t.Fatalf("SafeReference error %s", err)
	}
	ref, err := s.Get(ctx, &schema.Key{
		Key: []byte("key1"),
	})
	if err != nil {
		t.Fatalf("get error %s", err)
	}
	if !bytes.Equal(ref.Value, kv[0].Value) {
		t.Fatalf("safereference error")
	}
}

func testServerSafeReferenceError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.SafeReference(context.Background(), &schema.SafeReferenceOptions{
		Ro: &schema.ReferenceOptions{
			Key:       kv[0].Key,
			Reference: []byte("key1"),
		},
		RootIndex: &schema.Index{
			Index: 0,
		},
	})
	if err == nil {
		t.Fatalf("SafeReference exptected error")
	}
}

func testServerCount(ctx context.Context, s *ImmuServer, t *testing.T) {
	c, err := s.Count(ctx, &schema.KeyPrefix{
		Prefix: kv[0].Key,
	})
	if err != nil {
		t.Fatalf("Count error %s", err)
	}
	if c.Count == 0 {
		t.Fatalf("Count error >0 got %d", c.Count)
	}
}
func testServerCountError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.Count(context.Background(), &schema.KeyPrefix{
		Prefix: kv[0].Key,
	})
	if err == nil {
		t.Fatalf("Count expected error")
	}
}
func testServerPrintTree(ctx context.Context, s *ImmuServer, t *testing.T) {
	item, err := s.PrintTree(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("PrintTree  error %s", err)
	}
	if len(item.T) == 0 {
		t.Fatalf("PrintTree, expected > 0, got %v", len(item.T))
	}
}
func testServerPrintTreeError(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.PrintTree(context.Background(), &emptypb.Empty{})
	if err == nil {
		t.Fatalf("PrintTree exptected error")
	}
}

func testServerSetPermission(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.SetPermission(ctx, &schema.Item{
		Key:   []byte("key"),
		Value: []byte("val"),
		Index: 1,
	})
	if err == nil {
		t.Fatalf("SetPermission  fail")
	}
	if err == nil || !strings.Contains(err.Error(), "deprecated method. use change permission instead") {
		t.Fatalf("SetPermission  unexpected error: %s", err)
	}
}

func testServerDeactivateUserDeprecated(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.DeactivateUser(ctx, &schema.UserRequest{
		User: []byte("gerard"),
	})
	if err == nil {
		t.Fatalf("DeactivateUserDepricated  fail")
	}
	if err == nil || !strings.Contains(err.Error(), "deprecated method. use setactive instead") {
		t.Fatalf("DeactivateUserDepricated  unexpected error: %s", err)
	}
}

func testServerGetUser(ctx context.Context, s *ImmuServer, t *testing.T) {
	_, err := s.GetUser(ctx, &schema.UserRequest{
		User: []byte("gerard"),
	})
	if err == nil {
		t.Fatalf("GetUser  fail")
	}
	if err == nil || !strings.Contains(err.Error(), "deprecated method. use user list instead") {
		t.Fatalf("GetUser  unexpected error: %s", err)
	}
}

func TestServerUsermanagement(t *testing.T) {
	var err error
	s := newInmemoryAuthServer()
	ctx, err := login(s, auth.SysAdminUsername, auth.SysAdminPassword)
	if err != nil {
		log.Fatal(err)
	}
	newdb := &schema.Database{
		Databasename: testDatabase,
	}
	dbrepl, err := s.CreateDatabase(ctx, newdb)
	if err != nil {
		log.Fatal(err)
	}
	if dbrepl.Error.Errorcode != schema.ErrorCodes_Ok {
		log.Fatalf("error creating test database %v", dbrepl)
	}

	testServerCreateUser(ctx, s, t)
	testServerListDatabases(ctx, s, t)
	testServerUseDatabase(ctx, s, t)
	testServerChangePermission(ctx, s, t)
	testServerDeactivateUser(ctx, s, t)
	testServerSetActiveUser(ctx, s, t)
	testServerChangePassword(ctx, s, t)
	testServerListUsers(ctx, s, t)
	testServerSetPermission(ctx, s, t)
	testServerDeactivateUserDeprecated(ctx, s, t)
	testServerGetUser(ctx, s, t)
}
func TestServerDbOperations(t *testing.T) {
	var err error
	s := newInmemoryAuthServer()
	ctx, err := login(s, auth.SysAdminUsername, auth.SysAdminPassword)
	if err != nil {
		log.Fatal(err)
	}
	newdb := &schema.Database{
		Databasename: testDatabase,
	}
	dbrepl, err := s.CreateDatabase(ctx, newdb)
	if err != nil {
		log.Fatal(err)
	}
	if dbrepl.Error.Errorcode != schema.ErrorCodes_Ok {
		log.Fatalf("error creating test database %v", dbrepl)
	}
	testServerSetGet(ctx, s, t)
	testServerSetGetError(ctx, s, t)
	testServerSVSetGet(ctx, s, t)
	testServerSVSetGetError(ctx, s, t)
	testServerCurrentRoot(ctx, s, t)
	testServerCurrentRootError(ctx, s, t)
	testServerSafeSetGet(ctx, s, t)
	testServerSafeSetGetSV(ctx, s, t)
	testServerSafeSetGetSVError(ctx, s, t)
	testServerSetGetBatch(ctx, s, t)
	testServerSetGetBatchError(ctx, s, t)
	testServerSetGetBatchSV(ctx, s, t)
	testServerSetGetBatchSVError(ctx, s, t)
	testServerInclusion(ctx, s, t)
	testServerInclusionError(ctx, s, t)
	testServerConsintency(ctx, s, t)
	testServerConsintencyError(ctx, s, t)
	testServerByIndexSV(ctx, s, t)
	testServerByIndexSVError(ctx, s, t)
	testServerByIndex(ctx, s, t)
	testServerByIndexError(ctx, s, t)
	testServerHistory(ctx, s, t)
	testServerHistoryError(ctx, s, t)
	testServerBySafeIndex(ctx, s, t)
	testServerBySafeIndexError(ctx, s, t)
	testServerHealth(ctx, s, t)
	testServerHealthError(ctx, s, t)
	testServerHistorySV(ctx, s, t)
	testServerHistorySVError(ctx, s, t)
	testServerReference(ctx, s, t)
	testServerReferenceError(ctx, s, t)
	testServerZAdd(ctx, s, t)
	testServerZAddError(ctx, s, t)
	testServerScan(ctx, s, t)
	testServerScanError(ctx, s, t)
	testServerByIScanSV(ctx, s, t)
	testServerByIScanSVError(ctx, s, t)
	testServerPrintTree(ctx, s, t)
	testServerPrintTreeError(ctx, s, t)
	testServerScanSV(ctx, s, t)
	testServerScanSVError(ctx, s, t)
	testServerSafeReference(ctx, s, t)
	testServerSafeReferenceError(ctx, s, t)
	testServerCount(ctx, s, t)
	testServerCountError(ctx, s, t)
}

func TestServerUpdateConfigItem(t *testing.T) {
	dataDir := "test-server-update-config-item-config"
	configFile := fmt.Sprintf("%s.toml", dataDir)
	s := DefaultServer().WithOptions(DefaultOptions().
		WithCorruptionCheck(false).
		WithInMemoryStore(true).
		WithAuth(false).
		WithMaintenance(false).
		WithDir(dataDir).
		WithConfig(configFile))
	defer func() {
		os.RemoveAll(dataDir)
		os.Remove(configFile)
	}()

	// Config file path empty
	s.Options.Config = ""
	expectedErr := "config file does not exist"
	err := s.updateConfigItem("key", "key = value", func(string) bool { return false })
	require.Error(t, err)
	require.Equal(t, expectedErr, err.Error())
	s.Options.Config = configFile

	// ReadFile error
	immuOS := s.OS.(*immuos.StandardOS)
	readFileOK := immuOS.ReadFileF
	errReadFile := "ReadFile error"
	immuOS.ReadFileF = func(filename string) ([]byte, error) {
		return nil, errors.New(errReadFile)
	}
	expectedErr =
		fmt.Sprintf("error reading config file %s: %s", configFile, errReadFile)
	err = s.updateConfigItem("key", "key = value", func(string) bool { return false })
	require.Error(t, err)
	require.Equal(t, expectedErr, err.Error())
	immuOS.ReadFileF = readFileOK

	// Config already having the specified item
	ioutil.WriteFile(configFile, []byte("key = value"), 0644)
	expectedErr = "Server config already has key = value"
	err = s.updateConfigItem("key", "key = value", func(string) bool { return true })
	require.Error(t, err)
	require.Equal(t, expectedErr, err.Error())

	// Add new config item
	err = s.updateConfigItem("key2", "key2 = value2", func(string) bool { return false })
	require.NoError(t, err)

	// WriteFile error
	errWriteFile := errors.New("WriteFile error")
	immuOS.WriteFileF = func(filename string, data []byte, perm os.FileMode) error {
		return errWriteFile
	}
	err = s.updateConfigItem("key3", "key3 = value3", func(string) bool { return false })
	require.Equal(t, err, errWriteFile)
}

func TestServerUpdateAuthConfig(t *testing.T) {
	input, _ := ioutil.ReadFile("../../test/immudb.toml")
	err := ioutil.WriteFile("/tmp/immudb.toml", input, 0644)
	if err != nil {
		panic(err)
	}

	dataDir := "bratislava"
	s := DefaultServer().WithOptions(DefaultOptions().
		WithCorruptionCheck(false).
		WithInMemoryStore(true).
		WithAuth(false).
		WithMaintenance(false).WithDir(dataDir).WithConfig("/tmp/immudb.toml"))

	_, err = s.UpdateAuthConfig(context.Background(), &schema.AuthConfig{
		Kind: 1,
	})
	if err != nil {
		log.Fatal(err)
	}
	//TODO fix, after UpdateAuthConfig is fixed this should be uncommented
	// if !s.Options.GetAuth() {
	// 	log.Fatal("Error UpdateAuthConfig")
	// }
}

func TestServerUpdateMTLSConfig(t *testing.T) {
	input, _ := ioutil.ReadFile("../../test/immudb.toml")
	err := ioutil.WriteFile("/tmp/immudb.toml", input, 0644)
	if err != nil {
		panic(err)
	}

	dataDir := "ljubljana"
	s := DefaultServer().WithOptions(DefaultOptions().
		WithCorruptionCheck(false).
		WithInMemoryStore(true).
		WithAuth(false).
		WithMaintenance(false).WithDir(dataDir).WithMTLs(false).WithConfig("/tmp/immudb.toml"))
	_, err = s.UpdateMTLSConfig(context.Background(), &schema.MTLSConfig{
		Enabled: true,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func TestServerMtls(t *testing.T) {
	mtlsopts := MTLsOptions{
		Pkey:        "./../../test/mtls_certs/ca.key.pem",
		Certificate: "./../../test/mtls_certs/ca.cert.pem",
		ClientCAs:   "./../../test/mtls_certs/ca-chain.cert.pem",
	}
	op := DefaultOptions().
		WithCorruptionCheck(false).
		WithInMemoryStore(true).
		WithAuth(false).
		WithMaintenance(false).WithMTLs(true).WithMTLsOptions(mtlsopts)
	s := DefaultServer().WithOptions(op)
	ops, err := s.setUpMTLS()
	if err != nil {
		log.Fatal(err)
	}
	if len(ops) == 0 {
		log.Fatal("setUpMTLS expected options > 0")
	}

	// ReadFile error
	errReadFile := errors.New("ReadFile error")
	s.OS.(*immuos.StandardOS).ReadFileF = func(filename string) ([]byte, error) {
		return nil, errReadFile
	}
	_, err = s.setUpMTLS()
	require.Equal(t, errReadFile, err)
}

func TestServerPID(t *testing.T) {
	op := DefaultOptions().
		WithCorruptionCheck(false).
		WithInMemoryStore(true).
		WithAuth(false).
		WithMaintenance(false).WithPidfile("pidfile")
	s := DefaultServer().WithOptions(op)
	defer os.Remove("pidfile")
	err := s.setupPidFile()
	if err != nil {
		log.Fatal(err)
	}
}

func TestInsertNewUserAndOtherUserOperations(t *testing.T) {
	dataDir := "TestInsertNewUserAndOtherUserOperations"
	s := newAuthServer(dataDir)
	defer os.RemoveAll(dataDir)
	ctx, err := login(s, auth.SysAdminUsername, auth.SysAdminPassword)
	require.NoError(t, err)

	// insertNewUser errors
	_, _, err = s.insertNewUser([]byte("%"), nil, 1, DefaultdbName, true, auth.SysAdminUsername)
	require.Error(t, err)
	require.Contains(t, err.Error(), "username can only contain letters, digits and underscores")

	username := "someusername"
	usernameBytes := []byte(username)
	password := "$omePassword1"
	passwordBytes := []byte(password)
	_, _, err = s.insertNewUser(usernameBytes, []byte("a"), 1, DefaultdbName, true, auth.SysAdminUsername)
	require.Error(t, err)
	require.Contains(
		t,
		err.Error(),
		"password must have between 8 and 32 letters, digits and special characters "+
			"of which at least 1 uppercase letter, 1 digit and 1 special character")

	_, _, err = s.insertNewUser(usernameBytes, passwordBytes, 99, DefaultdbName, false, auth.SysAdminUsername)
	require.Error(t, err)
	require.Equal(t, err.Error(), "unknown permission")

	// getLoggedInUserDataFromUsername errors
	userdata := s.userdata.Userdata[username]
	delete(s.userdata.Userdata, username)
	_, err = s.getLoggedInUserDataFromUsername(username)
	require.Error(t, err)
	require.Equal(t, "Logedin user data not found", err.Error())
	s.userdata.Userdata[username] = userdata

	// getDbIndexFromCtx errors
	adminUserdata := s.userdata.Userdata[auth.SysAdminUsername]
	delete(s.userdata.Userdata, auth.SysAdminUsername)
	s.Options.maintenance = true
	_, err = s.getDbIndexFromCtx(ctx, "ListUsers")
	require.Error(t, err)
	require.Equal(t, "please select database first", err.Error())
	s.userdata.Userdata[auth.SysAdminUsername] = adminUserdata
	s.Options.maintenance = false

	// SetActiveUser errors
	_, err = s.SetActiveUser(ctx, &schema.SetActiveUserRequest{Username: "", Active: false})
	require.Error(t, err)
	require.Equal(t, "username can not be empty", err.Error())

	s.Options.auth = false
	_, err = s.SetActiveUser(ctx, &schema.SetActiveUserRequest{Username: username, Active: false})
	require.Error(t, err)
	require.Equal(t, "this command is available only with authentication on", err.Error())
	s.Options.auth = true

	delete(s.userdata.Userdata, auth.SysAdminUsername)
	_, err = s.SetActiveUser(ctx, &schema.SetActiveUserRequest{Username: username, Active: false})
	require.Error(t, err)
	require.Equal(t, "please login first", err.Error())
	s.userdata.Userdata[auth.SysAdminUsername] = adminUserdata

	_, err = s.SetActiveUser(ctx, &schema.SetActiveUserRequest{Username: auth.SysAdminUsername, Active: false})
	require.Error(t, err)
	require.Equal(t, "changing your own status is not allowed", err.Error())

	_, err = s.CreateUser(ctx, &schema.CreateUserRequest{
		User:       usernameBytes,
		Password:   passwordBytes,
		Permission: 1,
		Database:   DefaultdbName,
	})
	require.NoError(t, err)
	ctx2, err := login(s, username, password)
	require.NoError(t, err)
	_, err = s.SetActiveUser(ctx2, &schema.SetActiveUserRequest{Username: auth.SysAdminUsername, Active: false})
	require.Error(t, err)
	require.Equal(t, "user is not system admin nor admin in any of the databases", err.Error())

	_, err = s.SetActiveUser(ctx, &schema.SetActiveUserRequest{Username: "nonexistentuser", Active: false})
	require.Error(t, err)
	require.Equal(t, "user nonexistentuser not found", err.Error())
}

func TestGetUserAndUserExists(t *testing.T) {
	dataDir := "Test-Get-User"
	s := newAuthServer(dataDir)
	defer os.RemoveAll(dataDir)
	ctx, err := login(s, auth.SysAdminUsername, auth.SysAdminPassword)
	require.NoError(t, err)
	username := "someuser"
	_, err = s.CreateUser(ctx, &schema.CreateUserRequest{
		User:       []byte(username),
		Password:   []byte("Somepass1$"),
		Permission: 1,
		Database:   DefaultdbName})
	require.NoError(t, err)
	require.NoError(t, err)
	_, err = s.getUser([]byte(username), false)
	require.Error(t, err)
	require.Equal(t, "user not found", err.Error())

	_, err = s.userExists([]byte(username), []byte("wrongpass"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid user or password")
}

func TestIsAllowedDbName(t *testing.T) {
	err := IsAllowedDbName("")
	require.Error(t, err)
	require.Equal(t, "database name length outside of limits", err.Error())

	err = IsAllowedDbName(strings.Repeat("a", 33))
	require.Error(t, err)
	require.Equal(t, "database name length outside of limits", err.Error())

	err = IsAllowedDbName(" ")
	require.Error(t, err)
	require.Equal(t, "unrecognized character in database name", err.Error())

	err = IsAllowedDbName("-")
	require.Error(t, err)
	require.Equal(t, "punctuation marks and symbols are not allowed in database name", err.Error())

	err = IsAllowedDbName(strings.Repeat("a", 32))
	require.NoError(t, err)
}

func TestServerMandatoryAuth(t *testing.T) {
	dataDir := "Test-Server-Mandatory-Auth"
	s := newAuthServer(dataDir)
	defer os.RemoveAll(dataDir)
	s.Options.maintenance = true
	require.False(t, s.mandatoryAuth())

	s.Options.maintenance = false
	ctx, err := login(s, auth.SysAdminUsername, auth.SysAdminPassword)
	require.NoError(t, err)
	_, err = s.CreateUser(ctx, &schema.CreateUserRequest{
		User:       []byte("someuser"),
		Password:   []byte("Somepass1$"),
		Permission: 1,
		Database:   DefaultdbName,
	})
	require.NoError(t, err)
	s.dbList.Append(s.dbList.GetByIndex(0))
	require.True(t, s.mandatoryAuth())

	s.sysDb = nil
	require.True(t, s.mandatoryAuth())
}
