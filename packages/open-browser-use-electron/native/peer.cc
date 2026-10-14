#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <node_api.h>
#include <unistd.h>
#include <sys/socket.h>
#include <stdio.h>
#include <string>
#include <fstream>
#include <sstream>
#ifdef __APPLE__
#include <sys/un.h>
#include <sys/proc_info.h>
#include <libproc.h>
#include <bsm/libbsm.h>
#endif

static bool identity(int pid, unsigned* uid, std::string* start) {
#ifdef __APPLE__
  struct proc_bsdinfo info = {};
  if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info)) return false;
  *uid = info.pbi_uid;
  *start = std::to_string(info.pbi_start_tvsec) + ":" + std::to_string(info.pbi_start_tvusec);
  return true;
#elif defined(__linux__)
  std::ifstream stat("/proc/" + std::to_string(pid) + "/stat");
  std::string line;
  if (!std::getline(stat, line)) return false;
  auto close = line.rfind(')');
  if (close == std::string::npos) return false;
  std::istringstream fields(line.substr(close + 2));
  std::string field;
  for (int i = 3; i <= 22; i++) if (!(fields >> field)) return false;
  *start = field;
  std::ifstream status("/proc/" + std::to_string(pid) + "/status");
  while (std::getline(status, line)) {
    if (line.rfind("Uid:", 0) == 0) {
      unsigned real;
      std::istringstream uids(line.substr(4));
      return bool(uids >> real >> *uid);
    }
  }
  return false;
#else
  return false;
#endif
}

static napi_value result(napi_env env, int pid, unsigned uid, const std::string& start) {
  napi_value out, value;
  napi_create_object(env, &out);
  napi_create_int32(env, pid, &value); napi_set_named_property(env, out, "pid", value);
  napi_create_uint32(env, uid, &value); napi_set_named_property(env, out, "uid", value);
  napi_create_string_utf8(env, start.c_str(), start.size(), &value);
  napi_set_named_property(env, out, "startTime", value);
  return out;
}

static bool argument(napi_env env, napi_callback_info info, int* number) {
  size_t argc = 1; napi_value arg;
  if (napi_get_cb_info(env, info, &argc, &arg, nullptr, nullptr) != napi_ok || argc != 1 ||
      napi_get_value_int32(env, arg, number) != napi_ok || *number < 0) {
    napi_throw_type_error(env, "AUTH_UNAVAILABLE", "Expected a non-negative descriptor or PID");
    return false;
  }
  return true;
}

static napi_value process_identity(napi_env env, napi_callback_info info) {
  int pid; unsigned uid; std::string start;
  if (!argument(env, info, &pid)) return nullptr;
  if (!identity(pid, &uid, &start)) {
    napi_throw_error(env, "AUTH_UNAVAILABLE", "Cannot read registered process identity"); return nullptr;
  }
  return result(env, pid, uid, start);
}

static napi_value peer_identity(napi_env env, napi_callback_info info) {
  int fd, pid = -1; unsigned uid = 0; std::string start;
  if (!argument(env, info, &fd)) return nullptr;
#ifdef __APPLE__
  audit_token_t token; socklen_t length = sizeof(token);
  if (getsockopt(fd, SOL_LOCAL, LOCAL_PEERTOKEN, &token, &length) == 0 && length == sizeof(token)) {
    pid = audit_token_to_pid(token); uid = audit_token_to_euid(token);
  }
#elif defined(__linux__)
  struct ucred cred; socklen_t length = sizeof(cred);
  if (getsockopt(fd, SOL_SOCKET, SO_PEERCRED, &cred, &length) == 0 && length == sizeof(cred)) {
    pid = cred.pid; uid = cred.uid;
  }
#endif
  unsigned current_uid;
  if (pid <= 0 || !identity(pid, &current_uid, &start) || uid != current_uid) {
    napi_throw_error(env, "AUTH_UNAVAILABLE", "Cannot verify kernel socket peer"); return nullptr;
  }
  return result(env, pid, uid, start);
}

static napi_value init(napi_env env, napi_value exports) {
  napi_property_descriptor methods[] = {
    {"processIdentity", nullptr, process_identity, nullptr, nullptr, nullptr, napi_default, nullptr},
    {"peerIdentity", nullptr, peer_identity, nullptr, nullptr, nullptr, napi_default, nullptr}
  };
  napi_define_properties(env, exports, 2, methods); return exports;
}
NAPI_MODULE(NODE_GYP_MODULE_NAME, init)
