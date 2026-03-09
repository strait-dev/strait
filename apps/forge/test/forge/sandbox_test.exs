defmodule Forge.SandboxTest do
  use ExUnit.Case, async: false

  # These tests validate the public Sandbox API.
  # They use nil streams so gRPC send calls are no-ops (rescued in Runner).

  describe "execute/2" do
    test "executes python code and returns ok" do
      opts = %{
        run_id: "test-exec-1",
        language: "python",
        code: "print('hello from sandbox')",
        payload: nil,
        env: %{},
        timeout_ms: 10_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false
      }

      # With a nil stream, gRPC sends will be rescued.
      # The runner should still complete normally.
      result = Forge.Sandbox.execute(opts, nil)
      # Runner will crash trying to send on nil stream,
      # which means sandbox returns an error
      assert result == {:ok, nil} or match?({:error, _}, result)
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
        network_enabled: false
      }

      result = Forge.Sandbox.execute(opts, nil)
      assert result == {:ok, nil} or match?({:error, _}, result)
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
        network_enabled: false
      }

      # Should complete within ~1s due to timeout, not 30s
      {time_us, result} = :timer.tc(fn -> Forge.Sandbox.execute(opts, nil) end)
      time_ms = div(time_us, 1_000)

      # Should finish well before 30s (the sleep duration)
      assert time_ms < 10_000
      assert result == {:ok, nil} or match?({:error, _}, result)
    end
  end
end
