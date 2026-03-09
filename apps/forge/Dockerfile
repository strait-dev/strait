# Build stage
FROM elixir:1.17-alpine AS builder

RUN apk add --no-cache build-base git

WORKDIR /build

ENV MIX_ENV=prod

RUN mix local.hex --force && mix local.rebar --force

COPY apps/forge/mix.exs apps/forge/mix.lock* ./
RUN mix deps.get --only prod && mix deps.compile

COPY apps/forge/config/config.exs apps/forge/config/prod.exs config/
COPY apps/forge/lib lib/

RUN mix compile && mix release

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache libstdc++ openssl ncurses-libs python3 \
    && addgroup -S app \
    && adduser -S app -G app

WORKDIR /app

COPY --from=builder /build/_build/prod/rel/forge ./

USER app

EXPOSE 50051

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD ["bin/forge", "eval", "IO.puts(:ok)"]

ENTRYPOINT ["bin/forge"]
CMD ["start"]
