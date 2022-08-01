macro_rules! make_metric {
    (@labels_count $head:ident,) => {
        1usize
    };

    (@labels_count $head:ident, $($tail:ident,)+) => {
        1usize + make_metric!(@labels_count $($tail, )+)
    };

    (@gen_struct $name:ident) => {
        #[derive(Clone, Copy, Default)]
        pub struct $name {}
    };

    (@gen_struct $name:ident $name_with_labels:ident $(, $label:ident)+) => {
        impl $name {
            $(
                pub fn $label<V: std::string::ToString>(self, v: V) -> $name_with_labels {
                    $name_with_labels::init_with(stringify!($label), v)
                }
             )+
        }

        #[derive(Clone)]
        pub struct $name_with_labels {
            labels: Vec<(&'static str, String)>,
        }

        impl $name_with_labels {
            fn init_with<V: std::string::ToString>(name: &'static str, val: V) -> Self {
                #![allow(clippy::vec_init_then_push)]
                let mut labels = Vec::with_capacity(make_metric!(@labels_count $($label,)+));
                labels.push((name, val.to_string()));
                Self {
                    labels,
                }
            }
        }

        impl $name_with_labels {
            $(
                pub fn $label<V: std::string::ToString>(mut self, v: V) -> Self {
                    self.labels.push((stringify!($label), v.to_string()));
                    self
                }
             )+
        }
    };

    (@gen_methods counter $name:ident, $key:literal $(, $labels:ident)?) => {
        pub fn incr(self) {
            metrics::increment_counter!($key $(, &self.$labels[..])?);
        }

        pub fn incr_by(self, v: u64) {
            metrics::counter!($key, v $(, &self.$labels[..])?);
        }

        pub fn set_abs(self, v: u64) {
            metrics::absolute_counter!($key, v $(, &self.$labels[..])?);
        }
    };

    (@gen_methods gauge $name:ident, $key:literal $(, $labels:ident)?) => {
        pub fn incr<F: metrics::IntoF64>(self, v: F) {
            metrics::increment_gauge!($key, v $(, &self.$labels[..])?);
        }

        pub fn decr<F: metrics::IntoF64>(self, v: F) {
            metrics::decrement_gauge!($key, v $(, &self.$labels[..])?);
        }

        pub fn set<F: metrics::IntoF64>(self, v: F) {
            metrics::gauge!($key, v $(, &self.$labels[..])?);
        }
    };

    (@gen_methods histogram $name:ident, $key:literal $(, $labels:ident)?) => {
        pub fn record<F: metrics::IntoF64>(self, v: F) {
            metrics::histogram!($key, v $(, &self.$labels[..])?);
        }
    };

    ($mty:ident, $name:ident, $key:literal) => {
        make_metric!(@gen_struct $name);
        impl $name {
            make_metric!(@gen_methods $mty $name, $key);
        }
    };

    ($mty:ident, $name:ident, $key:literal $(, $label:ident)+) => {
        make_metric!(counter, $name, $key);
        paste::paste! {
            make_metric!(@gen_struct $name [<$name WithLabels>] $(, $label)+);

            impl [<$name WithLabels>] {
                make_metric!(@gen_methods $mty [<$name WithLabels>], $key, labels);
            }
        }
    };

    (@gen_view $(($field_name:ident: $type_name:ident, $mty:ident, $key:literal $(, $label:ident)*) ,)+) => {
        $(
            make_metric!($mty, $type_name, $key $(, $label)*);
         )+

        #[derive(Clone, Copy, Default)]
        pub struct View {
            $(
                pub $field_name: $type_name,
             )+
        }

        impl View {
            const fn new() -> Self {
                Self {
                    $(
                        $field_name: $type_name{},
                     )+
                }
            }
        }

        pub static VIEW: View = View::new();
    };

    ($(($name:ident: $mty:ident, $key:literal $(, $label:ident)*) ,)+) => {
        paste::paste! {
            make_metric!(@gen_view $(($name: [<$name:camel>], $mty, $key $(, $label)*) ,)+);
        }
    };
}

pub(super) use make_metric;
