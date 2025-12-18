use std::env;
use std::path::PathBuf;

fn main() {
    // Point to your proto files
    let proto_files = &["../../build/stack/bazel/bzlmod/v1/bcr.proto"];
    let includes = &["../../"];

    // Generate Rust code from proto files
    prost_build::Config::new()
        .out_dir(env::var("OUT_DIR").unwrap())
        .compile_protos(proto_files, includes)
        .expect("Failed to compile protos");
}
