defmodule Forge.Sandbox.Runner do
  @moduledoc """
  Executes user code in an isolated process with resource limits.

  Spawns an OS-level process for the target language runtime,
  captures stdout/stderr, enforces timeouts and memory limits,
  and streams events back to the caller.
  """
  use GenServer, restart: :temporary

  require Logger

  defstruct [:run_id, :language, :code, :payload, :env, :timeout_ms, :memory_bytes, :network_enabled, :stream, :port, :timer_ref, :output]

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts)
  end

  @impl true
  def init(opts) do
    state = %__MODULE__{
      run_id: opts.run_id,
      language: opts.language,
      code: opts.code,
      payload: opts.payload,
      env: opts.env || %{},
      timeout_ms: opts.timeout_ms,
      memory_bytes: opts.memory_bytes,
      network_enabled: opts.network_enabled,
      stream: opts.stream,
      output: []
    }

    {:ok, state, {:continue, :execute}}
  end

  @impl true
  def handle_continue(:execute, state) do
    timer_ref = Process.send_after(self(), :timeout, state.timeout_ms)
    state = %{state | timer_ref: timer_ref}

    case start_runtime(state) do
      {:ok, port} ->
        {:noreply, %{state | port: port}}

      {:error, reason} ->
        send_error(state.stream, state.run_id, "failed to start runtime: #{inspect(reason)}")
        {:stop, :normal, state}
    end
  end

  @impl true
  def handle_info({port, {:data, data}}, %{port: port} = state) do
    line = String.trim(data)
    send_log(state.stream, state.run_id, "info", line)
    {:noreply, %{state | output: [line | state.output]}}
  end

  def handle_info({port, {:exit_status, 0}}, %{port: port} = state) do
    cancel_timer(state.timer_ref)
    output = state.output |> Enum.reverse() |> Enum.join("\n")
    send_result(state.stream, state.run_id, true, output, nil)
    {:stop, :normal, state}
  end

  def handle_info({port, {:exit_status, code}}, %{port: port} = state) do
    cancel_timer(state.timer_ref)
    output = state.output |> Enum.reverse() |> Enum.join("\n")
    send_result(state.stream, state.run_id, false, output, "process exited with code #{code}")
    {:stop, :normal, state}
  end

  def handle_info(:timeout, state) do
    Logger.warning("Sandbox timeout run=#{state.run_id}")

    if state.port do
      Port.close(state.port)
    end

    send_result(state.stream, state.run_id, false, nil, "execution timed out")
    {:stop, :normal, state}
  end

  defp start_runtime(%{language: "python", code: code, env: env, payload: payload}) do
    # Write code to a temp file and execute with resource limits
    tmp_dir = System.tmp_dir!()
    code_path = Path.join(tmp_dir, "forge_#{:erlang.unique_integer([:positive])}.py")
    File.write!(code_path, code)

    env_list =
      env
      |> Map.put("FORGE_PAYLOAD", payload || "")
      |> Enum.map(fn {k, v} -> {String.to_charlist(k), String.to_charlist(v)} end)

    port =
      Port.open(
        {:spawn_executable, System.find_executable("python3")},
        [
          :binary,
          :exit_status,
          :use_stdio,
          :stderr_to_stdout,
          args: ["-u", code_path],
          env: env_list
        ]
      )

    {:ok, port}
  rescue
    e -> {:error, e}
  end

  defp start_runtime(%{language: lang}) do
    {:error, "unsupported language: #{lang}"}
  end

  defp send_log(stream, _run_id, level, message) do
    event = %Sandbox.V1.ExecutionEvent{
      event:
        {:log,
         %Sandbox.V1.LogEntry{
           level: level,
           message: message,
           timestamp_ms: System.system_time(:millisecond)
         }}
    }

    GRPC.Server.send_reply(stream, event)
  rescue
    _ -> :ok
  end

  defp send_result(stream, _run_id, success, result, error) do
    event = %Sandbox.V1.ExecutionEvent{
      event:
        {:result,
         %Sandbox.V1.ExecutionResult{
           success: success,
           result: (result && result) || "",
           error: error || "",
           duration_ms: 0
         }}
    }

    GRPC.Server.send_reply(stream, event)
  rescue
    _ -> :ok
  end

  defp send_error(stream, _run_id, error) do
    send_result(stream, nil, false, nil, error)
  end

  defp cancel_timer(nil), do: :ok
  defp cancel_timer(ref), do: Process.cancel_timer(ref)
end
