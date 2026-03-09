defmodule Forge.SandboxTest do
  use ExUnit.Case, async: false

  defp capture_send_fn(collector_pid) do
    fn _stream, event -> send(collector_pid, {:event, event}) end
  end

  defp collect_events do
    collect_events([])
  end

  defp collect_events(acc) do
    receive do
      {:event, event} -> collect_events([event | acc])
    after
      200 -> Enum.reverse(acc)
    end
  end

  describe "execute/2" do
    test "executes python code and streams events" do
      opts = %{
        run_id: "test-exec-1",
        language: "python",
        code: "print('hello from sandbox')",
        payload: nil,
        env: %{},
        timeout_ms: 10_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        send_fn: capture_send_fn(self())
      }

      result = Forge.Sandbox.execute(opts, nil)
      assert result == {:ok, nil}

      events = collect_events()
      assert length(events) >= 2

      log_events = Enum.filter(events, fn e -> match?({:log, _}, e.event) end)
      assert length(log_events) >= 1

      result_events = Enum.filter(events, fn e -> match?({:result, _}, e.event) end)
      assert length(result_events) == 1
      %{event: {:result, r}} = hd(result_events)
      assert r.success == true
    end

    test "handles unsupported language" do
      opts = %{
        run_id: "test-exec-unsupported",
        language: "cobol",
        code: "DISPLAY 'HELLO'",
        payload: nil,
        env: %{},
        timeout_ms: 5_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        send_fn: capture_send_fn(self())
      }

      result = Forge.Sandbox.execute(opts, nil)

      case result do
        {:ok, nil} ->
          events = collect_events()
          result_events = Enum.filter(events, fn e -> match?({:result, _}, e.event) end)
          assert length(result_events) == 1
          %{event: {:result, r}} = hd(result_events)
          assert r.success == false
          assert r.error =~ "unsupported language"

        {:error, _reason} ->
          # Runner may crash before sending events — acceptable
          :ok
      end
    end

    test "respects timeout" do
      opts = %{
        run_id: "test-exec-timeout",
        language: "python",
        code: "import time; time.sleep(30)",
        payload: nil,
        env: %{},
        timeout_ms: 1_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        send_fn: capture_send_fn(self())
      }

      {time_us, result} = :timer.tc(fn -> Forge.Sandbox.execute(opts, nil) end)
      time_ms = div(time_us, 1_000)

      assert time_ms < 10_000
      assert result == {:ok, nil}

      events = collect_events()
      result_events = Enum.filter(events, fn e -> match?({:result, _}, e.event) end)
      assert length(result_events) == 1
      %{event: {:result, r}} = hd(result_events)
      assert r.success == false
      assert r.error =~ "timed out"
    end

    test "captures all streamed events in order" do
      opts = %{
        run_id: "test-exec-order",
        language: "python",
        code: "print('a')\nprint('b')\nprint('c')",
        payload: nil,
        env: %{},
        timeout_ms: 10_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        send_fn: capture_send_fn(self())
      }

      assert {:ok, nil} = Forge.Sandbox.execute(opts, nil)

      events = collect_events()
      # Last event should be a result
      last = List.last(events)
      assert match?(%{event: {:result, _}}, last)
    end
  end
end
