use std::{marker::PhantomData, pin::Pin};

use bytes::{Buf, Bytes, BytesMut};

use super::{Deserializer, Serializer};

pub struct Json<Item, SinkItem> {
    _maker: PhantomData<(fn(SinkItem), fn() -> Item)>,
}

impl<Item, SinkItem> Default for Json<Item, SinkItem> {
    fn default() -> Self {
        Self {
            _maker: Default::default(),
        }
    }
}

impl<Item, SinkItem> Deserializer<SinkItem> for Json<Item, SinkItem>
where
    for<'a> SinkItem: serde::Deserialize<'a>,
{
    type DeserializeError = serde_json::Error;

    fn deserialize(self: Pin<&mut Self>, src: &BytesMut) -> Result<SinkItem, Self::DeserializeError> {
        serde_json::from_reader(std::io::Cursor::new(src).reader())
    }
}

impl<Item, SinkItem> Serializer<Item> for Json<Item, SinkItem>
where
    Item: serde::Serialize,
{
    type SerializeError = serde_json::Error;

    fn serialize(self: Pin<&mut Self>, item: &Item) -> Result<Bytes, Self::SerializeError> {
        serde_json::to_vec(item).map(Into::into)
    }
}
