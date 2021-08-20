use jsonrpc_core::Params;
use serde_json::Value;

use crate::channel::{oneshot, Sender};
use crate::RpcResult;

pub(crate) struct CallMessage {
    pub(crate) method: String,
    pub(crate) params: Params,
    pub(crate) sender: oneshot::Tx<RpcResult<Value>>,
}

pub(crate) struct NotifyMessage {
    pub(crate) method: String,
    pub(crate) params: Params,
}

pub(crate) struct Subscription {
    pub(crate) subscribe: String,
    pub(crate) subscribe_params: Params,
    pub(crate) notification: String,
    pub(crate) unsubscribe: String,
}

pub(crate) struct SubscribeMessage {
    pub(crate) subscription: Subscription,
    pub(crate) sender: Sender<RpcResult<Value>>,
}

pub(crate) enum RpcMessage {
    Call(CallMessage),
    Notify(NotifyMessage),
    Subscribe(SubscribeMessage),
}

impl From<CallMessage> for RpcMessage {
    fn from(msg: CallMessage) -> Self {
        RpcMessage::Call(msg)
    }
}

impl From<NotifyMessage> for RpcMessage {
    fn from(msg: NotifyMessage) -> Self {
        RpcMessage::Notify(msg)
    }
}

impl From<SubscribeMessage> for RpcMessage {
    fn from(msg: SubscribeMessage) -> Self {
        RpcMessage::Subscribe(msg)
    }
}
