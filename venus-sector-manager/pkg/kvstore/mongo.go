package kvstore

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
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

var (
	_ KVStore = (*MongoStore)(nil)
	_ Iter    = (*MongoIter)(nil)
	_ DB      = (*mongoDB)(nil)
	_ Txn     = (*MongoTxn)(nil)
)

type MongoStore struct {
	client *mongo.Client
	coll   *mongo.Collection
}

func (ms MongoStore) View(ctx context.Context, f func(Txn) error) error {
	opts := options.Session().SetDefaultReadConcern(readconcern.Majority())
	sess, err := ms.client.StartSession(opts)
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		txn := &MongoTxn{
			sessCtx: sessCtx,
			coll:    ms.coll,
		}
		return nil, f(txn)
	})
	return err
}

func (ms MongoStore) Update(ctx context.Context, f func(Txn) error) error {
	opts := options.Session().SetDefaultWriteConcern(writeconcern.New(writeconcern.WMajority()))
	sess, err := ms.client.StartSession(opts)
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		txn := &MongoTxn{
			sessCtx: sessCtx,
			coll:    ms.coll,
		}
		return nil, f(txn)
	})
	return err
}

func (ms MongoStore) NeedRetryTransactions() bool {
	return false
}

func (ms MongoStore) Get(ctx context.Context, key Key) (Val, error) {
	return get(ms.coll, ctx, key)
}

func (ms MongoStore) Peek(ctx context.Context, key Key, f func(Val) error) error {
	v := KvInMongo{}
	err := ms.coll.FindOne(ctx, bson.M{"_id": KeyToString(key)}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return ErrKeyNotFound
	}
	if err != nil {
		return err
	}
	return f(v.Val)
}

func (ms MongoStore) Put(ctx context.Context, key Key, val Val) error {
	return put(ms.coll, ctx, key, val)
}

func (ms MongoStore) Del(ctx context.Context, key Key) error {
	return del(ms.coll, ctx, key)
}

func (ms MongoStore) Scan(ctx context.Context, prefix Prefix) (Iter, error) {
	return scan(ms.coll, ctx, prefix)
}

type MongoTxn struct {
	sessCtx mongo.SessionContext
	coll    *mongo.Collection
}

func (mt *MongoTxn) Get(key Key) (Val, error) {
	return get(mt.coll, mt.sessCtx, key)
}
func (mt *MongoTxn) Peek(key Key, f func(Val) error) error {
	v := KvInMongo{}
	err := mt.coll.FindOne(mt.sessCtx, bson.M{"_id": KeyToString(key)}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return ErrKeyNotFound
	}
	if err != nil {
		return err
	}
	return f(v.Val)
}

func (mt *MongoTxn) Put(key Key, val Val) error {
	return put(mt.coll, mt.sessCtx, key, val)
}

func (mt *MongoTxn) Del(key Key) error {
	return del(mt.coll, mt.sessCtx, key)
}

func (mt *MongoTxn) Scan(prefix Prefix) (Iter, error) {
	return scan(mt.coll, mt.sessCtx, prefix)
}

func get(coll *mongo.Collection, ctx context.Context, key Key) (Val, error) {
	v := KvInMongo{}
	err := coll.FindOne(ctx, bson.M{"_id": KeyToString(key)}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return nil, ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return v.Val, nil
}

func put(coll *mongo.Collection, ctx context.Context, key Key, val Val) error {
	_, err := coll.UpdateOne(ctx, bson.M{"_id": KeyToString(key)}, bson.M{"$set": KvInMongo{
		Key:    KeyToString(key),
		RawKey: key,
		Val:    val,
	}}, &options.UpdateOptions{
		Upsert: &Upsert,
	})
	return err
}

func del(coll *mongo.Collection, ctx context.Context, key Key) error {
	_, err := coll.DeleteOne(ctx, bson.M{"_id": KeyToString(key)})
	return err
}

func scan(coll *mongo.Collection, ctx context.Context, prefix Prefix) (Iter, error) {
	s := KeyToString(prefix)
	s = "^" + s
	cur, err := coll.Find(ctx, bson.M{"_id": primitive.Regex{
		Pattern: s,
		Options: "i",
	}})
	if err != nil {
		return nil, err
	}
	return &MongoIter{cur: cur}, nil
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

func (m *MongoIter) View(ctx context.Context, f func(Val) error) error {
	if m.data == nil {
		return fmt.Errorf("wrong usage of View, should call next first")
	}
	return f(m.data.Val)
}

func (m *MongoIter) Close() {
	m.cur.Close(context.TODO())
}

type mongoDB struct {
	client *mongo.Client
	dbName string
}

func (db mongoDB) Run(context.Context) error {
	return nil
}

func (db mongoDB) Close(context.Context) error {
	return nil
}

func (db mongoDB) OpenCollection(_ context.Context, name string) (KVStore, error) {
	return &MongoStore{
		client: db.client,
		coll:   db.client.Database(db.dbName).Collection(name),
	}, nil
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

	return &mongoDB{
		client: client,
		dbName: dbName,
	}, nil
}
