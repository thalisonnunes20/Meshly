# Meshly

> Uma alternativa moderna, aberta e auto-hospedável ao **Radmin VPN** e **Hamachi**, movida pelo protocolo ultrarrápido **WireGuard**.

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)
![Wails](https://img.shields.io/badge/Wails-v2-DF1A5E.svg)

---

## Principais Recursos

- **Zero Configuração Manual:** O usuário final apenas baixa o programa, informa o servidor e o apelido, e cria ou entra em salas com senha.
- **Motor WireGuard Embutido:** Sem necessidade de instalar o cliente oficial do WireGuard separadamente.
- **Isolamento Rígido entre Redes:** Redes isoladas matematicamente via sub-redes (`26.0.X.0/24`) e regras de firewall `iptables` no servidor. Quem está na Sala A jamais enxerga ou pinga a Sala B.
- **Painel Web de Administração com Login:** Interface web moderna e protegida por autenticação para gerenciar redes, alterar senhas, visualizar status e expulsar usuários em tempo real.
- **Persistência de Sessão:** O aplicativo desktop memoriza o servidor, apelido e todas as redes salvas do usuário, reconectando automaticamente ao abrir.
- **Medição Silenciosa de Latência:** Teste de ping contínuo em milissegundos sem abrir janelas ou interferir no sistema operacional.
- **Split-Tunneling Nativo:** Apenas o tráfego da rede virtual passa pelo túnel, mantendo a internet padrão e downloads na velocidade máxima.
- **Interface Minimalista e Moderna:** Tema escuro sóbrio (zinc/slate), sem poluição visual ou emojis, com ícones vetoriais de alta precisão.

---

## Arquitetura do Repositório (Monorepo)

```text
Meshly/
├── server/                     # Servidor de Controle e WireGuard (Go)
│   ├── cmd/server/             # Ponto de entrada do executável
│   ├── internal/
│   │   ├── api/                # REST API, WebSockets e Painel Web Admin
│   │   ├── config/             # Carregador de configurações via variáveis de ambiente
│   │   ├── store/              # Armazenamento em memória com senhas em bcrypt
│   │   ├── wireguard/          # Gestor de interface wg0 e regras iptables
│   │   └── ws/                 # Hub WebSocket para presença e status em tempo real
│   ├── Dockerfile              # Imagem Docker multi-stage Debian Slim
│   └── docker-compose.yml      # Configuração pronta para VPS
├── client/                     # Aplicativo Desktop (Windows .exe / Linux)
│   ├── app.go                  # Lógica Go de túnel, chaves Curve25519 e persistência
│   ├── main.go                 # Configurações de janela Wails e runtime
│   ├── wails.json              # Configuração de build
│   └── frontend/src/           # Interface gráfica moderna (HTML5, Vanilla CSS, JS)
└── docker-compose.yml          # Compose na raiz para inicialização rápida
```

---

## 1. Servidor (Container Docker)

### Pré-requisitos na VPS / VM:
- Docker e Docker Compose instalados.
- Portas liberadas no firewall da máquina:
  - `8080/TCP` (API REST, WebSockets e Painel Web Admin).
  - `51820/UDP` (Túnel criptografado WireGuard).

### Inicialização Rápida:
```bash
git clone https://github.com/thalisonnunes20/Meshly.git
cd Meshly
docker compose up -d --build
```

### Acompanhar Logs:
```bash
docker compose logs -f
```

### Variáveis de Ambiente Disponíveis (`docker-compose.yml`):
| Variável | Padrão | Descrição |
| :--- | :--- | :--- |
| `MESHLY_API_PORT` | `8080` | Porta interna da API e Painel Web |
| `MESHLY_WG_PORT` | `51820` | Porta UDP do túnel WireGuard |
| `MESHLY_ADMIN_USER` | `admin` | Usuário de acesso ao Painel Web |
| `MESHLY_ADMIN_PASSWORD` | `admin123` | Senha de acesso ao Painel Web |
| `MESHLY_PUBLIC_ENDPOINT` | *(auto)* | IP público ou domínio da VPS (ex: `vpn.seusite.com`) |

### Personalizar Portas:
Se desejar rodar a API em outra porta externa (ex: `9000` ou `80`), basta alterar o mapeamento no `docker-compose.yml`:
```yaml
ports:
  - "9000:8080"       # Agora acessível em http://SEU_IP:9000
  - "51820:51820/udp"
```

---

## 2. Painel Web de Administração

Acesse pelo navegador em:
```text
http://SEU_IP_OU_DOMINIO:8080/
```

- **Usuário Padrão:** `admin`
- **Senha Padrão:** `admin123`

### Recursos do Painel:
- Visualização de todas as redes virtuais ativas e sub-redes alocadas (`26.0.X.0/24`).
- Tabela detalhada de membros: apelido, endereço IPv4 virtual, chave pública e status online/offline.
- **Expulsar Usuário:** Desconecta e revoga o acesso do membro da rede imediatamente.
- **Mudar Senha:** Atualiza a senha da rede em tempo real.
- **Excluir Rede:** Deleta a rede e encerra os túneis associados.

---

## 3. Cliente Desktop (Windows / Linux)

O cliente é compilado utilizando **Wails v2**, gerando um executável nativo leve (~12 MB), sem dependência de navegadores externos pesados ou instaladores de terceiros.

### Executável Pré-Compilado (Windows):
O executável compilado pronto para uso pode ser gerado diretamente na pasta:
```text
Meshly/client/build/bin/meshly.exe
```

### Como Compilar o Cliente:
1. Instale o Wails CLI caso ainda não possua:
   ```bash
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```
2. Entre na pasta do cliente e compile:
   ```bash
   cd client
   wails build -s
   ```
   *O executável nativo otimizado será gerado em `build/bin/meshly.exe` (Windows) ou `build/bin/meshly` (Linux).*

### Dados e Persistência do Usuário:
O aplicativo salva automaticamente as credenciais e redes salvas em:
- **Windows:** `%APPDATA%\Meshly\config.json`
- **Linux:** `~/.meshly/config.json`

---

## Licença

Distribuído sob a licença **MIT**. Veja o arquivo [LICENSE](LICENSE) para mais informações.
