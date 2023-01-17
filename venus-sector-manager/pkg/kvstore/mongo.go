package kvstore

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	mlog   = Log.With("driver", "mongo")
	Upsert = true
)

func KeyToString(k Key) string {
	return string(k)
}

// KvInMongo represent the struct stored in db.
type KvInMongo struct {
	Key    string `bson:"_id"`
	Val    Val    `bson:"val"`
	RawKey Key    `bson:"raw"`
}

var _ KVStore = (*MongoStore)(nil)
var _ Iter = (*MongoIter)(nil)
var _ DB = (*mongoDB)(nil)

type MongoStore struct {
	col *mongo.Collection
}

type MongoIter struct {
	cur  *mongo.Cursor
	data *KvInMongo
}

func (m *MongoIter) Next() bool {
	next := m.cur.Next(context.TODO())
	if !next {
		return next
	}
	k := &KvInMongo{}
	err := m.cur.Decode(k)
	if err != nil {
		mlog.Error(fmt.Errorf("decode data from cursor failed: %w", err))
		return false
	}
	m.data = k
	return true
}

func (m *MongoIter) Key() Key {
	if m.data == nil {
		mlog.Error("wrong usage of KEY, should call next first")
		return nil
	}
	return m.data.RawKey
}

func (m *MongoIter) View(ctx context.Context, callback Callback) error {
	if m.data == nil {
		return fmt.Errorf("wrong usage of View, should call next first")
	}
	return callback(m.data.Val)
}

func (m *MongoIter) Close() {
	m.cur.Close(context.TODO())
}

func (m MongoStore) Get(ctx context.Context, key Key) (Val, error) {
	v := Val{}
	err := m.View(ctx, key, func(val Val) error {
		v = val
		return nil
	})
	return v, err
}

func (m MongoStore) Has(ctx context.Context, key Key) (bool, error) {
	count, err := m.col.CountDocuments(ctx, bson.D{{Key: "_id", Value: KeyToString(key)}})
	return count > 0, err
}

func (m MongoStore) View(ctx context.Context, key Key, callback Callback) error {
	v := KvInMongo{}
	err := m.col.FindOne(ctx, bson.M{"_id": KeyToString(key)}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return ErrKeyNotFound
	}
	if err != nil {
		return err
	}

	return callback(v.Val)
}

func (m MongoStore) Put(ctx context.Context, key Key, val Val) error {
	_, err := m.col.UpdateOne(ctx, bson.M{"_id": KeyToString(key)}, bson.M{"$set": KvInMongo{
		Key:    KeyToString(key),
		RawKey: key,
		Val:    val,
	}}, &options.UpdateOptions{
		Upsert: &Upsert,
	})
	return err
}

func (m MongoStore) Del(ctx context.Context, key Key) error {
	_, err := m.col.DeleteOne(ctx, bson.M{"_id": KeyToString(key)})
	return err
}

func (m MongoStore) Scan(ctx context.Context, prefix Prefix) (Iter, error) {
	s := KeyToString(prefix)
	s = "^" + s
	cur, err := m.col.Find(ctx, bson.M{"_id": primitive.Regex{
		Pattern: s,
		Options: "i",
	}})
	if err != nil {
		return nil, err
	}
	return &MongoIter{cur: cur}, nil
}

type mongoDB struct {
	inner *mongo.Database
}

func (db mongoDB) Run(context.Context) error {
	return nil
}

func (db mongoDB) Close(context.Context) error {
	return nil
}

func (db mongoDB) OpenCollection(_ context.Context, name string) (KVStore, error) {
	return MongoStore{col: db.inner.Collection(name)}, nil
}

func OpenMongo(ctx context.Context, dsn string, dbName string) (DB, error) {
	client, err := mongo.NewClient(options.Client().ApplyURI(dsn).SetAppName("venus-cluster"))
	if err != nil {
		err = fmt.Errorf("new mongo client %s: %w", dsn, err)
		return nil, err
	}

	if err = client.Connect(ctx); err != nil {
		err = fmt.Errorf("connect to %s: %w", dsn, err)
		return nil, err
	}

	return &mongoDB{inner: client.Database(dbName)}, nil
}
