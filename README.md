# Meshly 🌐

> Uma alternativa moderna, aberta e auto-hospedável ao **Radmin VPN** e **Hamachi**, movida pelo protocolo ultrarrápido **WireGuard**.

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)

---

## ⚡ Principais Recursos

- **Zero Configuração de Arquivos:** O usuário final apenas baixa o programa, digita o servidor e o apelido, e cria/entra em salas com senha.
- **Motor WireGuard Embutido:** Sem necessidade de instalar o WireGuard oficial separadamente.
- **Isolamento Total de Salas:** Redes isoladas matematicamente via sub-redes (`26.0.X.0/24`) e regras de firewall `iptables` no servidor. Quem está na Sala A não enxerga a Sala B.
- **Presença em Tempo Real:** WebSockets integrados para avisar instantaneamente quem entrou, saiu ou está offline.
- **Split-Tunneling Nativo:** Apenas o tráfego da rede virtual passa pelo túnel, sem alterar a internet padrão ou deixar os jogos lentos.
- **Interface Desktop Moderna:** Visual autêntico em Dark Mode, com medição de ping ICMP, cópia rápida de IP e árvore hierárquica de salas.

---

## 🏗️ Arquitetura do Repositório

```text
Meshly/
├── server/                 # Servidor de Controle e WireGuard (Go)
│   ├── cmd/server/         # Ponto de entrada do executável
│   ├── internal/
│   │   ├── api/            # Endpoints REST (/health, /create, /join, /leave)
│   │   ├── config/         # Configurações via variáveis de ambiente
│   │   ├── store/          # Armazenamento em memória com senhas bcrypt
│   │   ├── wireguard/      # Gestor de interface wg0 e regras iptables
│   │   └── ws/             # Hub WebSocket para presença em tempo real
│   ├── Dockerfile          # Imagem de produção Debian Slim
│   └── docker-compose.yml
├── client/                 # Aplicativo Desktop (Windows .exe / Linux)
│   ├── core/               # Lógica de conexão e interface de rede Wintun/TUN
│   └── ui/                 # Interface gráfica moderna (sem cara de IA)
└── docker-compose.yml      # Compose na raiz para inicialização rápida
```

---

## 🚀 Como Subir o Servidor (Debian / VPS)

### Pré-requisitos na Máquina/VM:
- Docker e Docker Compose instalados
- Portas liberadas no firewall da VM/VPS:
  - `8080/TCP` (API e WebSockets)
  - `51820/UDP` (Túnel WireGuard)

### 1. Clonar e Iniciar:
```bash
git clone https://github.com/thalisonnunes20/Meshly.git
cd Meshly
docker compose up -d --build
```

### 2. Verificar se está rodando:
```bash
# Ver os logs em tempo real:
docker compose logs -f

# Testar a API de saúde no navegador ou terminal:
curl http://localhost:8080/api/v1/health
```

---

## 📄 Licença

Distribuído sob a licença MIT. Veja `LICENSE` para mais informações.
