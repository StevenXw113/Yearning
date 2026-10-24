#!/usr/bin/env bash
# 生成引擎 gRPC 通道的 TLS / mTLS 证书（本地私有 CA，测试与中小部署够用）。
# 用法: ./scripts/gen-tls-certs.sh [输出目录] [服务器名]
#   输出目录   默认 ./certs
#   服务器名   主程序连引擎用的主机名/IP，写入服务端证书 SAN（默认 127.0.0.1）
#
# 产物: ca.crt / server.crt / server.key / client.crt / client.key
# 部署: 引擎侧挂 server.*（-tls-cert/-tls-key），启 mTLS 再挂 ca.crt（-tls-client-ca）；
#       主程序侧挂 ca.crt + client.*（[General].RpcTLSCA/RpcTLSCert/RpcTLSKey）。
# 生产建议用企业 CA 签发，本脚本适合内网快速起步；私钥文件务必限制读取权限（600）。
set -euo pipefail

DIR="${1:-./certs}"
SERVER_NAME="${2:-127.0.0.1}"
mkdir -p "$DIR"
cd "$DIR"

echo "==> 生成私有 CA"
openssl ecparam -name prime256v1 -genkey -noout -out ca.key
openssl req -x509 -new -key ca.key -sha256 -days 3650 -subj "/CN=yearning-engine-ca" -out ca.crt

echo "==> 生成服务端证书 (SAN: ${SERVER_NAME})"
openssl ecparam -name prime256v1 -genkey -noout -out server.key
openssl req -new -key server.key -subj "/CN=yearning-engine" -out server.csr
cat > server-ext.cnf <<EOF
subjectAltName = DNS:${SERVER_NAME},IP:127.0.0.1
extendedKeyUsage = serverAuth
EOF
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 825 -sha256 \
  -extfile server-ext.cnf -out server.crt

echo "==> 生成客户端证书（主程序用）"
openssl ecparam -name prime256v1 -genkey -noout -out client.key
openssl req -new -key client.key -subj "/CN=yearning-main" -out client.csr
cat > client-ext.cnf <<EOF
extendedKeyUsage = clientAuth
EOF
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 825 -sha256 \
  -extfile client-ext.cnf -out client.crt

chmod 600 ./*.key
rm -f ./*.csr server-ext.cnf client-ext.cnf ca.srl
echo "==> 完成: $DIR/{ca,server,client}.{crt,key}"
echo "   引擎:   ./engine -tls-cert ${DIR}/server.crt -tls-key ${DIR}/server.key \\"
echo "           [-tls-client-ca ${DIR}/ca.crt   # 加上即 mTLS]"
echo "   主程序: [General] RpcTLSCA=${DIR}/ca.crt RpcTLSCert=${DIR}/client.crt RpcTLSKey=${DIR}/client.key"
