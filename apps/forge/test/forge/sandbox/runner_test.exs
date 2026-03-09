defmodule Forge.Sandbox.RunnerTest do
  use ExUnit.Case, async: true

  alias Forge.Sandbox.Runner

  defp capture_send_fn(collector_pid) do
    fn _stream, event -> send(collector_pid, {:event, event}) end
  end

  defp base_opts(overrides \\ %{}) do
    Map.merge(
      %{
        run_id: "test-run-1",
        language: "python",
        code: "print('hello')",
        payload: nil,
        env: %{},
        timeout_ms: 10_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        stream: nil,
        send_fn: capture_send_fn(self())
      },
      overrides
    )
  end

  describe "start_link/1" do
    test "starts a runner process that exits normally" do
      opts = base_opts()
      assert {:ok, pid} = Runner.start_link(opts)
      assert is_pid(pid)
      ref = Process.monitor(pid)

      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000
    end

    test "streams log events for stdout lines" do
      opts = base_opts(%{code: "print('line1')\nprint('line2')"})
      {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)

      # Collect events until process exits
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000

      events = collect_events()
      log_events = Enum.filter(events, fn e -> match?({:log, _}, e.event) end)
      assert length(log_events) >= 1

      all_messages =
        log_events
        |> Enum.map(fn %{event: {:log, log}} -> log.message end)
        |> Enum.join("\n")

      assert all_messages =~ "line1"
      assert all_messages =~ "line2"
    end

    test "streams result event with success=true on exit 0" do
      opts = base_opts(%{code: "print('ok')"})
      {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000

      events = collect_events()
      result_events = Enum.filter(events, fn e -> match?({:result, _}, e.event) end)
      assert length(result_events) == 1

      %{event: {:result, result}} = hd(result_events)
      assert result.success == true
      assert result.error == ""
    end

    test "streams result event with success=false on non-zero exit" do
      opts = base_opts(%{code: "import sys; sys.exit(1)"})
      {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000

      events = collect_events()
      result_events = Enum.filter(events, fn e -> match?({:result, _}, e.event) end)
      assert length(result_events) == 1

      %{event: {:result, result}} = hd(result_events)
      assert result.success == false
      assert result.error =~ "exited with code"
    end

    test "tracks duration_ms correctly" do
      opts = base_opts(%{code: "import time; time.sleep(0.3)"})
      {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000

      events = collect_events()
      [%{event: {:result, result}}] = Enum.filter(events, fn e -> match?({:result, _}, e.event) end)
      assert result.duration_ms >= 200
    end

    test "passes environment variables to process" do
      opts = base_opts(%{
        code: "import os; print(os.environ.get('MY_VAR', 'NOT_SET'))",
        env: %{"MY_VAR" => "hello_world"}
      })
      {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000

      events = collect_events()
      log_events = Enum.filter(events, fn e -> match?({:log, _}, e.event) end)
      messages = Enum.map(log_events, fn %{event: {:log, log}} -> log.message end)
      assert "hello_world" in messages
    end

    test "passes payload via FORGE_PAYLOAD env var" do
      opts = base_opts(%{
        code: "import os; print(os.environ.get('FORGE_PAYLOAD', 'NONE'))",
        payload: ~s({"key":"value"})
      })
      {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000

      events = collect_events()
      log_events = Enum.filter(events, fn e -> match?({:log, _}, e.event) end)
      messages = Enum.map(log_events, fn %{event: {:log, log}} -> log.message end)
      assert Enum.any?(messages, fn m -> m =~ "key" end)
    end

    test "cleans up temp file after success" do
      # Create a unique marker to identify our temp file
      marker = ":erlang.unique_integer([:positive])"
      code = "print('cleanup #{marker}')"
      opts = base_opts(%{code: code})

      # Count forge temp files before
      before_count =
        System.tmp_dir!()
        |> File.ls!()
        |> Enum.count(fn f -> String.starts_with?(f, "forge_") and String.ends_with?(f, ".py") end)

      {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 10_000

      # After execution, count should not have grown
      after_count =
        System.tmp_dir!()
        |> File.ls!()
        |> Enum.count(fn f -> String.starts_with?(f, "forge_") and String.ends_with?(f, ".py") end)

      assert after_count <= before_count
    end

    test "runner stops with error for unsupported language" do
      opts = base_opts(%{language: "rust", code: "fn main() {}"})
      assert {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 5_000

      events = collect_events()
      result_events = Enum.filter(events, fn e -> match?({:result, _}, e.event) end)
      assert length(result_events) == 1

      %{event: {:result, result}} = hd(result_events)
      assert result.success == false
      assert result.error =~ "unsupported language"
    end
  end

  defp collect_events do
    collect_events([])
  end

  defp collect_events(acc) do
    receive do
      {:event, event} -> collect_events([event | acc])
    after
      100 -> Enum.reverse(acc)
    end
  end
end
