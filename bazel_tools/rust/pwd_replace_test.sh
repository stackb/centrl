env
# set -x

for key in $(env | cut -d= -f1); do
    current="${!key}"
    # echo "envkey: $key"
    # echo "envval: $val"
    if [[ "${current}" == *"\${pwd}"* ]]; then
        next="${val//\$\{pwd\}/$PWD}"
        # # export key
        # echo "matched for $key:"
        # echo "next: $next"
        
        # # ="$next"
        export "${key}=${next}"
        # echo "--------"
        # # env
        # echo "CARGO_MANIFEST_DIR:${CARGO_MANIFEST_DIR}"
        # exit 1
    fi
done

if [[ "${CARGO_MANIFEST_DIR}" != "$PWD/foo" ]]; then
    exit 1
fi