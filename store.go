package etcdstore

import (
	"context"
	"encoding/base32"
	"errors"
	"github.com/coreos/etcd/clientv3"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
	"strings"
)

type EtcdStore struct {
	Options *sessions.Options
	Codecs  []securecookie.Codec

	client *clientv3.Client
}

func New(config clientv3.Config, path string, maxAge int, keyPairs ...[]byte) (*EtcdStore, error) {
	client, err := clientv3.New(config)
	if err != nil {
		return nil, err
	}

	return &EtcdStore{
		client: client,
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   path,
			MaxAge: maxAge,
		},
	}, nil
}

func (s *EtcdStore) Close() {
	s.client.Close()
}

// Get should return a cached session.
func (s *EtcdStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

// New should create and return a new session.
//
// Note that New should never return a nil session, even in the case of
// an error if using the Registry infrastructure to cache the session.
func (s *EtcdStore) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(s, name)
	session.IsNew = true
	opts := *s.Options
	session.Options = &opts
	session.IsNew = true
	var err error
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if err == nil {
			err = s.load(session)
			if err == nil {
				session.IsNew = false
			} else {
				err = nil
			}
		}
	}

	return session, err
}

// Save adds a single session to the response.
func (s *EtcdStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	if session.ID == "" {
		session.ID = strings.TrimRight(
			base32.StdEncoding.EncodeToString(
				securecookie.GenerateRandomKey(32),
			),
			"=",
		)
	}

	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values, s.Codecs...)
	if err != nil {
		return err
	}

	_, err = s.client.Put(context.TODO(), session.ID, encoded)
	if err != nil {
		return err
	}

	encoded, err = securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
	if err != nil {
		return err
	}

	http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

func (s *EtcdStore) load(session *sessions.Session) error {
	ses, err := s.client.Get(context.TODO(), session.ID)
	if err != nil {
		return err
	}

	if len(ses.Kvs) < 1 {
		return errors.New("Kvs empty")
	}

	data := string(ses.Kvs[0].Value)
	return securecookie.DecodeMulti(session.Name(), data, &session.Values, s.Codecs...)
}
