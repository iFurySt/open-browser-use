{
  "targets": [{
    "target_name": "iab_peer",
    "sources": ["native/peer.cc"],
    "defines": ["NAPI_VERSION=8"],
    "conditions": [["OS=='mac'", {"libraries": ["-lbsm"]}]]
  }]
}
