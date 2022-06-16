use byte_unit::Byte;

/// CPU information
pub mod cpu;
/// Disk information
pub mod disk;
/// GPU information
pub mod gpu;
/// Memory information
pub mod mem;

/// Format the bytes to string.
pub fn byte_string(bytes: u64, fractional_digits: usize) -> String {
    Byte::from(bytes)
        .get_appropriate_unit(true)
        .format(fractional_digits)
}
