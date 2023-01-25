use std::{error::Error as StdError, io};

use bytes::{Bytes, BytesMut};
use futures::{Sink, Stream};
use tokio_util::codec::{Decoder, Encoder, LengthDelimitedCodec};

use crate::serded::Serded;

pub mod lines_codec;
pub use lines_codec::*;

impl<T: ?Sized> Framed for T where T: Sink<Bytes> + Stream<Item = Result<BytesMut, Self::Error>> {}

pub trait Framed: Sink<Bytes> + Stream<Item = Result<BytesMut, Self::Error>> {}

impl<T: ?Sized> FramedExt for T where T: Framed {}

pub trait FramedExt: Framed {
    fn serded<Codec, Item, SinkItem>(self, serde_codec: Codec) -> Serded<Self, Codec, Item, SinkItem>
    where
        Self: Sized,
    {
        Serded::new(self, serde_codec)
    }
}

pub enum DynCodec {
    Lines(LinesCodec),
    LengthDelimited(LengthDelimitedCodec),
}

impl DynCodec {
    pub fn new(imp: impl AsRef<str>) -> io::Result<Self> {
        match imp.as_ref() {
            "lines" => Ok(Self::Lines(LinesCodec::new())),
            "length_delimited" => Ok(Self::LengthDelimited(LengthDelimitedCodec::new())),
            _ => Err(io::Error::new(
                io::ErrorKind::Unsupported,
                format!("unsupported codec {}", imp.as_ref()),
            )),
        }
    }
}

impl Decoder for DynCodec {
    type Item = BytesMut;

    type Error = Box<dyn StdError>;

    fn decode(&mut self, src: &mut BytesMut) -> Result<Option<Self::Item>, Self::Error> {
        match self {
            DynCodec::Lines(lines) => lines.decode(src).map_err(|e| Box::new(e) as Box<dyn StdError>),
            DynCodec::LengthDelimited(length_delimited) => length_delimited.decode(src).map_err(|e| Box::new(e) as Box<dyn StdError>),
        }
    }
}

impl Encoder<Bytes> for DynCodec {
    type Error = Box<dyn StdError>;

    fn encode(&mut self, item: Bytes, dst: &mut BytesMut) -> Result<(), Self::Error> {
        match self {
            DynCodec::Lines(lines) => lines.encode(item, dst).map_err(|e| Box::new(e) as Box<dyn StdError>),
            DynCodec::LengthDelimited(length_delimited) => length_delimited.encode(item, dst).map_err(|e| Box::new(e) as Box<dyn StdError>),
        }
    }
}
