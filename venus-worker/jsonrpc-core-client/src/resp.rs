use jsonrpc_core::{Error, Id, Params};
use jsonrpc_pubsub::SubscriptionId;
use serde::{Deserialize, Serialize};
use serde_json::Value;

use crate::RpcError;

pub fn parse_response(
    response: &str,
) -> Result<
    (
        Id,
        Result<Value, RpcError>,
        Option<String>,
        Option<SubscriptionId>,
    ),
    RpcError,
> {
    jsonrpc_core::serde_from_str::<ClientResponse>(response)
        .map_err(|e| RpcError::ParseError(e.to_string(), Box::new(e)))
        .map(|response| {
            let id = response.id().unwrap_or(Id::Null);
            let sid = response.subscription_id();
            let method = response.method();
            let value: Result<Value, Error> = response.into();
            let result = value.map_err(RpcError::JsonRpcError);
            (id, result, method, sid)
        })
}

/// A type representing all possible values sent from the server to the client.
#[derive(Debug, PartialEq, Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
#[serde(untagged)]
pub enum ClientResponse {
    /// A regular JSON-RPC request output (single response).
    Output(jsonrpc_core::Output),
    /// A notification.
    Notification(jsonrpc_core::Notification),
}

impl ClientResponse {
    /// Get the id of the response (if any).
    pub fn id(&self) -> Option<Id> {
        match *self {
            ClientResponse::Output(ref output) => Some(output.id().clone()),
            ClientResponse::Notification(_) => None,
        }
    }

    /// Get the method name if the output is a notification.
    pub fn method(&self) -> Option<String> {
        match *self {
            ClientResponse::Notification(ref n) => Some(n.method.to_owned()),
            ClientResponse::Output(_) => None,
        }
    }

    /// Parses the response into a subscription id.
    pub fn subscription_id(&self) -> Option<SubscriptionId> {
        match *self {
            ClientResponse::Notification(ref n) => match &n.params {
                jsonrpc_core::Params::Map(map) => match map.get("subscription") {
                    Some(value) => SubscriptionId::parse_value(value),
                    None => None,
                },
                _ => None,
            },
            _ => None,
        }
    }
}

impl From<ClientResponse> for Result<Value, Error> {
    fn from(res: ClientResponse) -> Self {
        match res {
            ClientResponse::Output(output) => output.into(),
            ClientResponse::Notification(n) => match &n.params {
                Params::Map(map) => {
                    let subscription = map.get("subscription");
                    let result = map.get("result");
                    let error = map.get("error");

                    match (subscription, result, error) {
                        (Some(_), Some(result), _) => Ok(result.to_owned()),
                        (Some(_), _, Some(error)) => {
                            let error = serde_json::from_value::<Error>(error.to_owned())
                                .ok()
                                .unwrap_or_else(Error::parse_error);
                            Err(error)
                        }
                        _ => Ok(n.params.into()),
                    }
                }
                _ => Ok(n.params.into()),
            },
        }
    }
}
