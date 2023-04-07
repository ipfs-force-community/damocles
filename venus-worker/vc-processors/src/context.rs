use std::{
    fmt,
    num::ParseIntError,
    time::{Duration, SystemTime, UNIX_EPOCH},
};

use opentelemetry_api::trace::TraceContextExt;
use serde::{Deserialize, Serialize};
use tracing_opentelemetry::OpenTelemetrySpanExt;

#[derive(Serialize, Deserialize, Clone, Debug)]
pub struct Context {
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub trace_flags: TraceFlags,
    /// Unix timestamp in seconds. When the client expects the request to be complete by. The server should cancel the request
    /// if it is not complete by this time.
    pub deadline: u64,
}

impl Context {
    /// Returns the context for the current request, or a default Context if no request is active.
    pub fn current() -> Self {
        let span = tracing::Span::current();
        (&span).into()
    }

    pub fn deadline(&self) -> SystemTime {
        SystemTime::UNIX_EPOCH
            .checked_add(Duration::from_secs(self.deadline))
            .unwrap_or(SystemTime::UNIX_EPOCH)
    }
}

#[derive(Clone)]
struct Deadline(u64);

impl Default for Deadline {
    fn default() -> Self {
        Deadline(one_minute_from_now())
    }
}

/// An extension trait for [`tracing::Span`] for propagating Contexts.
pub(crate) trait SpanExt {
    /// Sets the given context on this span. Newly-created spans will be children of the given
    /// context's trace context.
    fn set_context(&self, context: &Context);
}

impl SpanExt for tracing::Span {
    fn set_context(&self, context: &Context) {
        self.set_parent(
            opentelemetry_api::Context::new()
                .with_remote_span_context(opentelemetry_api::trace::SpanContext::new(
                    opentelemetry_api::trace::TraceId::from(context.trace_id),
                    opentelemetry_api::trace::SpanId::from(context.span_id),
                    opentelemetry_api::trace::TraceFlags::from(context.trace_flags),
                    true,
                    opentelemetry_api::trace::TraceState::default(),
                ))
                .with_value(Deadline(context.deadline)),
        );
    }
}

impl From<&tracing::Span> for Context {
    fn from(span: &tracing::Span) -> Self {
        let context = span.context();
        let span_ref = context.span();
        let otel_ctx = span_ref.span_context();
        Self {
            trace_id: TraceId::from(otel_ctx.trace_id()),
            span_id: SpanId::from(otel_ctx.span_id()),
            trace_flags: TraceFlags::from(otel_ctx.trace_flags()),
            deadline: context.get::<Deadline>().cloned().unwrap_or_default().0,
        }
    }
}

/// A 16-byte value which identifies a given trace.
///
/// The id is valid if it contains at least one non-zero byte.
#[derive(Clone, PartialEq, Eq, Copy, Hash, Serialize, Deserialize)]
pub struct TraceId(#[serde(with = "u128_serde")] u128);

impl TraceId {
    /// Invalid trace id
    pub const INVALID: TraceId = TraceId(0);

    /// Create a trace id from its representation as a byte array.
    pub const fn from_bytes(bytes: [u8; 16]) -> Self {
        TraceId(u128::from_be_bytes(bytes))
    }

    /// Return the representation of this trace id as a byte array.
    pub const fn to_bytes(self) -> [u8; 16] {
        self.0.to_be_bytes()
    }

    /// Converts a string in base 16 to a trace id.
    ///
    /// # Examples
    ///
    /// ```
    /// use vc_processors::context::TraceId;
    ///
    /// assert!(TraceId::from_hex("42").is_ok());
    /// assert!(TraceId::from_hex("58406520a006649127e371903a2de979").is_ok());
    ///
    /// assert!(TraceId::from_hex("not_hex").is_err());
    /// ```
    pub fn from_hex(hex: &str) -> Result<Self, ParseIntError> {
        u128::from_str_radix(hex, 16).map(TraceId)
    }
}

/// An 8-byte value which identifies a given span.
///
/// The id is valid if it contains at least one non-zero byte.
#[derive(Clone, PartialEq, Eq, Copy, Hash, Serialize, Deserialize)]
pub struct SpanId(u64);

impl SpanId {
    /// Invalid span id
    pub const INVALID: SpanId = SpanId(0);

    /// Create a span id from its representation as a byte array.
    pub const fn from_bytes(bytes: [u8; 8]) -> Self {
        SpanId(u64::from_be_bytes(bytes))
    }

    /// Return the representation of this span id as a byte array.
    pub const fn to_bytes(self) -> [u8; 8] {
        self.0.to_be_bytes()
    }

    /// Converts a string in base 16 to a span id.
    ///
    /// # Examples
    ///
    /// ```
    /// use vc_processors::context::SpanId;
    ///
    /// assert!(SpanId::from_hex("42").is_ok());
    /// assert!(SpanId::from_hex("58406520a0066491").is_ok());
    ///
    /// assert!(SpanId::from_hex("not_hex").is_err());
    /// ```
    pub fn from_hex(hex: &str) -> Result<Self, ParseIntError> {
        u64::from_str_radix(hex, 16).map(SpanId)
    }
}

/// Flags that can be set on a [`Context`].
///
/// The current version of the specification only supports a single flag
/// [`TraceFlags::SAMPLED`].
///
/// See the W3C TraceContext specification's [trace-flags] section for more
/// details.
///
/// [trace-flags]: https://www.w3.org/TR/trace-context/#trace-flags
#[derive(Clone, Debug, Default, PartialEq, Eq, Copy, Hash, Serialize, Deserialize)]
pub struct TraceFlags(u8);

impl TraceFlags {
    /// Trace flags with the `sampled` flag set to `1`.
    ///
    /// Spans that are not sampled will be ignored by most tracing tools.
    /// See the `sampled` section of the [W3C TraceContext specification] for details.
    ///
    /// [W3C TraceContext specification]: https://www.w3.org/TR/trace-context/#sampled-flag
    pub const SAMPLED: TraceFlags = TraceFlags(0x01);

    /// Construct new trace flags
    pub const fn new(flags: u8) -> Self {
        TraceFlags(flags)
    }

    /// Returns copy of the current flags with the `sampled` flag set.
    pub fn with_sampled(&self, sampled: bool) -> Self {
        Self::new(if sampled {
            self.to_u8() | TraceFlags::SAMPLED.to_u8()
        } else {
            self.to_u8() & !TraceFlags::SAMPLED.to_u8()
        })
    }

    /// Returns the flags as a `u8`
    pub fn to_u8(self) -> u8 {
        self.0
    }
}

impl From<opentelemetry_api::trace::TraceId> for TraceId {
    fn from(trace_id: opentelemetry_api::trace::TraceId) -> Self {
        Self::from_bytes(trace_id.to_bytes())
    }
}

impl From<TraceId> for opentelemetry_api::trace::TraceId {
    fn from(trace_id: TraceId) -> Self {
        Self::from_bytes(trace_id.to_bytes())
    }
}

impl fmt::Debug for TraceId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_fmt(format_args!("{:032x}", self.0))
    }
}

impl fmt::Display for TraceId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_fmt(format_args!("{:032x}", self.0))
    }
}

impl fmt::LowerHex for TraceId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::LowerHex::fmt(&self.0, f)
    }
}

impl fmt::Debug for SpanId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_fmt(format_args!("{:016x}", self.0))
    }
}

impl fmt::Display for SpanId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_fmt(format_args!("{:016x}", self.0))
    }
}

impl fmt::LowerHex for SpanId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::LowerHex::fmt(&self.0, f)
    }
}

impl From<opentelemetry_api::trace::SpanId> for SpanId {
    fn from(span_id: opentelemetry_api::trace::SpanId) -> Self {
        Self::from_bytes(span_id.to_bytes())
    }
}

impl From<SpanId> for opentelemetry_api::trace::SpanId {
    fn from(span_id: SpanId) -> Self {
        Self::from_bytes(span_id.to_bytes())
    }
}

impl From<TraceFlags> for opentelemetry_api::trace::TraceFlags {
    fn from(trace_flags: TraceFlags) -> Self {
        opentelemetry_api::trace::TraceFlags::new(trace_flags.to_u8())
    }
}

impl From<opentelemetry_api::trace::TraceFlags> for TraceFlags {
    fn from(trace_flags: opentelemetry_api::trace::TraceFlags) -> Self {
        TraceFlags::new(trace_flags.to_u8())
    }
}

impl From<&opentelemetry_api::trace::SpanContext> for TraceFlags {
    fn from(context: &opentelemetry_api::trace::SpanContext) -> Self {
        Self::new(context.trace_flags().to_u8())
    }
}

mod u128_serde {
    pub fn serialize<S>(u: &u128, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        serde::Serialize::serialize(&u.to_le_bytes(), serializer)
    }

    pub fn deserialize<'de, D>(deserializer: D) -> Result<u128, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        Ok(u128::from_le_bytes(serde::Deserialize::deserialize(deserializer)?))
    }
}

fn one_minute_from_now() -> u64 {
    (SystemTime::now() + Duration::from_secs(60))
        .duration_since(UNIX_EPOCH)
        .map(|x| x.as_secs())
        .unwrap_or(0)
}
