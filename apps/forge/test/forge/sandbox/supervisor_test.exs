defmodule Forge.Sandbox.SupervisorTest do
  use ExUnit.Case, async: true

  alias Forge.Sandbox.Supervisor, as: SandboxSupervisor

  describe "start_execution/1" do
    test "starts a child process under the supervisor" do
      opts = %{
        run_id: "test-sup-1",
        language: "python",
        code: "print('supervised')",
        payload: nil,
        env: %{},
        timeout_ms: 5_000,
        memory_bytes: 256 * 1024 * 1024,
        network_enabled: false,
        stream: nil
      }

      assert {:ok, pid} = SandboxSupervisor.start_execution(opts)
      assert is_pid(pid)
      assert Process.alive?(pid)
    end
  end
end
