# Flet para Go — Plano

Objetivo: uma lib em que a interface é escrita em **Go** e desenhada por um **cliente Flutter**, do mesmo jeito que o [Flet](https://github.com/flet-dev/flet) faz com Python.

---

## 1. Como funciona

```
┌──────────────────────┐      mensagens       ┌──────────────────────┐
│       Lado Go        │  ─── add/update ───▶ │    Cliente Flutter   │
│  árvore de controles │                      │  desenha os widgets  │
│  diff + handlers     │  ◀──── event ─────── │  envia eventos       │
└──────────────────────┘  WebSocket / socket  └──────────────────────┘
```

- O Go monta uma **árvore de controles** (Text, Button, Column…) na memória.
- O cliente Flutter só sabe desenhar essa árvore. É um "terminal gráfico" rico.
- **Go → Flutter:** adicionar, mudar propriedade, remover.
- **Flutter → Go:** "clicaram no botão X", "o campo Y mudou".
- Lógica e estado ficam todos no Go.

Fluxo de uma sessão:

1. O Go inicia um servidor local.
2. O Go abre o cliente e informa o endereço.
3. O cliente se conecta e recebe a árvore inicial.
4. A cada evento, o Go roda o handler e manda **só o que mudou**.

---

## 2. Decisão: qual cliente Flutter

> ✅ **Decidido: opção A** — reaproveitar o cliente do Flet, com o protocolo isolado em uma camada própria.


| | A. Reaproveitar o cliente do Flet | B. Cliente próprio |
|---|---|---|
| Velocidade do MVP | Rápida | Mais lenta |
| Widgets prontos | Muitos | Só os que fizermos |
| Precisa de Flutter agora | Não | Sim |
| Protocolo | O do Flet (interno, muda entre versões) | O nosso, simples e estável |
| Controle do projeto | Preso às decisões do Flet | Total |
| Trabalho por widget | Só o lado Go | Go + Dart |

**Recomendação:** começar pelo **A** para validar a ideia rápido, mas com o protocolo isolado em uma camada própria no Go. Assim, se o projeto ficar sério, dá para trocar pelo **B** sem reescrever a API que o desenvolvedor usa.

---

## 3. Lado Go (a lib)

- Cada controle é uma **struct** (Text, Button, Column…) com um **ID único**.
- A **Page** guarda a árvore atual.
- **Update da página:** compara com o último estado enviado e manda só a diferença.
- **Handlers:** um mapa de "ID + evento → função" para despachar o que chega do cliente.
- **Uma goroutine por sessão/conexão.**
- **Camadas separadas:**
  - **API:** controles e Page. É o que o desenvolvedor usa.
  - **Protocolo:** converte a árvore em mensagens.
  - **Transporte:** WebSocket ou socket local.

Exemplo-alvo: um contador com um texto e um botão "+1". O handler do clique soma 1, muda o texto e chama o update da página.

---

## 4. Protocolo

### Se usarmos o cliente do Flet (opção A)
- Seguimos o protocolo do Flet. Nas versões recentes, ele usa **MessagePack**. *A confirmar no código.*
- Biblioteca: **`github.com/vmihailenco/msgpack/v5`**: madura, popular, uso parecido com `encoding/json`.
- Fixar uma versão do cliente.

### Se fizermos cliente próprio (opção B)
- Começar com **JSON** (fácil de depurar) e trocar por **MessagePack** depois, se precisar.
- Quatro mensagens:

| Mensagem | Direção | Para quê |
|---|---|---|
| `add` | Go → Flutter | Inserir controles em um pai |
| `update` | Go → Flutter | Mudar propriedades de um controle |
| `remove` | Go → Flutter | Tirar controles da árvore |
| `event` | Flutter → Go | Clique, mudança de campo etc. |

- O cliente guarda um espelho da árvore (mapa ID → nó) e redesenha **só o widget afetado**.

---

## 5. Plataformas

### Web
- Só **web com servidor**: o binário Go serve o cliente Flutter web por WebSocket.
- **Sem web estático.**

### Desktop
- O Go abre o cliente e conversa por socket local.

### Mobile
- O Go é compilado como **biblioteca**: `.so` no Android e `.xcframework` no iOS.
- Ao abrir o app, o Flutter chama `Start()`, que sobe o servidor Go em uma goroutine.
- O cliente se conecta pelo socket local, igual ao desktop.
- Motivo: o iOS não deixa um app iniciar outro processo.

---

## 6. Riscos

- **Latência na web:** todo clique vai e volta no servidor. Com rede ruim, isso se sente. Animações e arrastar devem ficar no lado Flutter.
- **Memória por sessão:** cada usuário mantém uma árvore viva no servidor.
- **Reconexão:** é preciso tratar a queda e a volta da conexão sem perder o estado.
- **Mapeamento de widgets:** é o trabalho braçal. Na opção B, cada widget novo exige código em Go e em Dart.
- **Protocolo do Flet (opção A):** é interno e pode mudar entre versões.
- **Builds mobile:** Xcode, NDK e assinatura quebram por detalhes de ambiente.

---

## 7. Alternativas descartadas

| Lib | Por que não |
|---|---|
| **Fyne** | Go puro, mas o visual e a variedade de widgets ficam atrás do Flutter |
| **Gio** | Flexível, mas baixo nível. Daria muito mais trabalho para chegar no mesmo resultado |

Também não precisamos de WebAssembly nem de `syscall/js`.

---

## 8. Fases e estimativas

| Fase | Entrega | Opção A | Opção B |
|---|---|---|---|
| 1. Protótipo | Contador funcionando de ponta a ponta | 1 a 2 dias | 3 a 5 dias |
| 2. Controles | Conjunto principal de widgets | 1 a 2 sem. (gerador de código) | 3 a 6 sem. (Go + Dart) |
| 3. Diff, sessões, reconexão | Atualização eficiente e vários usuários | 1 a 2 sem. | 1 a 2 sem. |
| 4. Web com servidor | Binário Go servindo o app | 2 a 4 dias | 2 a 4 dias |
| 5. Android | App rodando no aparelho | 1 a 2 sem. | 1 a 2 sem. |
| 6. iOS | Idem, com xcframework | 1 a 2 sem. | 1 a 2 sem. |
| 7. CLI e empacotamento | Algo como `flet-go build` | 1 sem. | 1 sem. |

- **Opção A:** desktop e web em cerca de 1 mês; tudo em 2 a 3 meses.
- **Opção B:** desktop e web em cerca de 2 meses; tudo em 3 a 5 meses.

---

## 9. Protótipo (fase 1)

### O que ele faz
O contador: um texto e um botão "+1". Ao clicar, o número sobe.

Isso prova:
- Conexão com o cliente.
- Envio da árvore de controles.
- Recebimento de eventos.
- Envio só do que mudou.

### Ambiente

| Item | Situação |
|---|---|
| Go 1.27 | ✅ Instalado |
| Git | ✅ Instalado |
| Pasta do projeto | ✅ Criada |
| Flutter | ➖ Só na opção B |
| Repositório do Flet (estudo) | ⏳ Clonar fora do projeto |
| Cliente Flet para macOS | ⏳ Baixar (só opção A) |
| Biblioteca MessagePack | ⏳ Instalar (só opção A) |

### Etapas (opção A)

| Etapa | Tempo |
|---|---|
| Estudar o protocolo no código do Flet | 1 a 2 h |
| Conexão e envio dos controles | 2 a 3 h |
| Eventos e atualização da tela | 1 a 2 h |
| Ajustes até funcionar | 2 a 4 h |

---

## 10. Resultado do protótipo

✅ **Funcionou** com o cliente Flet v1.1.2 no macOS. O contador abre, e cada clique envia só a mudança do texto.

### Protocolo do cliente Flet (v1.1.2)

- **Transporte:** TCP local (`tcp://127.0.0.1:porta`) ou socket Unix. O endereço é o 1º argumento do cliente.
- **Pacote:** `[tamanho:u32 LE][tipo:u8][corpo]`
  - Tipo `0x00`: mensagem MessagePack `[ação, corpo]`.
  - Tipo `0x01`: canal de dados binário (não usado ainda).
- **Ações:**

| Código | Ação | Direção |
|---|---|---|
| 1 | Registrar cliente | Ida e volta |
| 2 | Patch de controle | Go → cliente |
| 3 | Evento de controle | Cliente → Go |
| 4 | Atualizar propriedades | Cliente → Go |
| 5 | Chamar método | Ida e volta |
| 6 | Sessão travou | Go → cliente |

- **Controle:** mapa com `_c` (tipo, ex.: `"Text"`), `_i` (id) e as propriedades. Eventos aparecem como `on_click: true`.
- **IDs reservados:** 1 = Page, 2 = Window. Os controles do Go começam no 3.
- **Registro:** o cliente manda tamanho, plataforma e tema. O Go responde com a página completa (`page_patch`).
- **Patch:** `[[árvore], operação, …]`. Operação de troca: `[0, nó, propriedade, valor]`. Também existem adicionar (1), remover (2) e mover (3).

### Estrutura atual

```
app.go              → abre o cliente e espera a conexão
page.go             → árvore, diff e envio das mudanças
session.go          → mensagens do cliente
control.go          → base dos controles e IDs
controls.go         → Text, Button, Column, Row
event.go            → eventos
internal/protocol/  → formato das mensagens
internal/client/    → abre o cliente (variável FLET_CLIENT_PATH)
examples/counter/   → o contador
```

A sessão ficou na raiz, e não em `internal/session`, para evitar dependência circular.

### Como rodar

1. Baixe e extraia o `flet-macos.tar.gz` da v1.1.2 do Flet.
2. Aponte `FLET_CLIENT_PATH` para o `Flet.app`.
3. Rode o exemplo `examples/counter`. Use `FLETGO_DEBUG=1` para ver as mensagens.

### Limitações conhecidas

- Quando uma lista de filhos muda, ela é enviada inteira (ainda sem adicionar/remover item a item).
- Controles removidos continuam no índice de eventos.
- Os eventos rodam um por vez, na ordem em que chegam.
- Sem reconexão.
- O cliente não é baixado automaticamente.

---

## 11. Próximos passos

1. ~~Decidir entre opção A e B~~ → opção A.
2. ~~Estudar o protocolo~~ → seção 10.
3. ~~Árvore de controles, IDs, diff e despacho de eventos~~.
4. ~~Contador funcionando de ponta a ponta~~.
5. Baixar o cliente automaticamente, como o Python faz.
6. Diff de listas item a item e limpeza do índice.
7. Fase 2: gerador de controles a partir das definições do Flet.
