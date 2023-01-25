use std::{io, marker::PhantomData, pin::Pin};

use bincode as bincode_crate;
use bincode_crate::Options;
use bytes::{Bytes, BytesMut};

use super::{Deserializer, Serializer};

pub struct Bincode<Item, SinkItem, O = bincode_crate::DefaultOptions> {
    options: O,
    _maker: PhantomData<(fn(SinkItem), fn() -> Item)>,
}

impl<Item, SinkItem> Default for Bincode<Item, SinkItem> {
    fn default() -> Self {
        Bincode {
            options: Default::default(),
            _maker: PhantomData,
        }
    }
}

impl<Item, SinkItem, O> Deserializer<SinkItem> for Bincode<Item, SinkItem, O>
where
    for<'a> SinkItem: serde::Deserialize<'a>,
    O: Options + Clone,
{
    type DeserializeError = io::Error;

    fn deserialize(self: Pin<&mut Self>, src: &BytesMut) -> Result<SinkItem, Self::DeserializeError> {
        self.options
            .clone()
            .deserialize(src)
            .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))
    }
}

impl<Item, SinkItem, O> Serializer<Item> for Bincode<Item, SinkItem, O>
where
    Item: serde::Serialize,
    O: Options + Clone,
{
    type SerializeError = io::Error;

    fn serialize(self: Pin<&mut Self>, item: &Item) -> Result<Bytes, Self::SerializeError> {
        self.options
            .clone()
            .serialize(item)
            .map(Into::into)
            .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))
    }
}
