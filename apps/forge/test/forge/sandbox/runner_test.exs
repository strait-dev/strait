defmodule Forge.Sandbox.RunnerTest do
  use ExUnit.Case, async: true

  alias Forge.Sandbox.Runner

  describe "start_link/1" do
    test "starts a runner process" do
      # Runner is :temporary restart, so it will stop after execution.
      # This test verifies the process can be started.
      opts = %{
        run_id: "test-run-1",
        language: "python",
        code: "print('hello')",
        payload: nil,
        env: %{},
        timeout_ms: 5_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        stream: nil
      }

      # Runner will fail because stream is nil (can't send gRPC replies),
      # but it should start without crashing the test process.
      assert {:ok, pid} = Runner.start_link(opts)
      assert is_pid(pid)
      ref = Process.monitor(pid)

      # Wait for it to finish (will stop normally or error)
      assert_receive {:DOWN, ^ref, :process, ^pid, _reason}, 10_000
    end
  end

  describe "unsupported language" do
    test "runner stops with error for unsupported language" do
      opts = %{
        run_id: "test-run-unsupported",
        language: "rust",
        code: "fn main() {}",
        payload: nil,
        env: %{},
        timeout_ms: 5_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        stream: nil
      }

      assert {:ok, pid} = Runner.start_link(opts)
      ref = Process.monitor(pid)
      assert_receive {:DOWN, ^ref, :process, ^pid, :normal}, 5_000
    end
  end
end
