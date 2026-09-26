import { useCallback, useEffect, useRef, useState } from 'react'
import { Modal, Spin, Result, Button } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import QRCode from 'qrcode'
import { api } from '../api/client'

type Phase = 'loading' | 'waiting' | 'scanned' | 'confirmed' | 'error' | 'expired'

interface Props {
  open: boolean
  onClose: () => void
  onSuccess: () => void
}

export default function QRLoginModal({ open, onClose, onSuccess }: Props) {
  const [phase, setPhase] = useState<Phase>('loading')
  const [qrDataUrl, setQrDataUrl] = useState('')
  const [errMsg, setErrMsg] = useState('')
  const [reloadKey, setReloadKey] = useState(0)
  const timerRef = useRef<number | null>(null)
  const aliveRef = useRef(false)

  const stopPolling = useCallback(() => {
    if (timerRef.current !== null) {
      clearInterval(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const createSession = useCallback(async () => {
    setPhase('loading')
    setErrMsg('')
    try {
      const { qrText } = await api.qrCreate()
      const url = await QRCode.toDataURL(qrText, { width: 240, margin: 1 })
      setQrDataUrl(url)
      setPhase('waiting')
    } catch (e) {
      setErrMsg((e as Error).message)
      setPhase('error')
    }
  }, [])

  // 打开时创建会话；关闭时停止轮询并作废会话
  useEffect(() => {
    if (!open) {
      aliveRef.current = false
      stopPolling()
      return
    }
    aliveRef.current = true
    createSession()
    return () => {
      aliveRef.current = false
      stopPolling()
    }
  }, [open, createSession, stopPolling])

  // 串行长轮询：get/status 是长轮询（未扫码时 hold ~30s），
  // 上一次返回后再发起下一次，避免请求堆积
  useEffect(() => {
    if (!open || (phase !== 'waiting' && phase !== 'scanned')) return
    let stopped = false
    const poll = async () => {
      while (!stopped && aliveRef.current) {
        try {
          const { status } = await api.qrStatus(40_000)
          if (stopped || !aliveRef.current) break
          if (status === 'confirmed') {
            setPhase('confirmed')
            onSuccess()
            return
          }
          if (status === 'expired' || status === 'canceled' || status === 'error') {
            setPhase('expired')
            return
          }
          setPhase(status)
        } catch {
          // 网络抖动：稍等重试
        }
        await new Promise((r) => setTimeout(r, 1000))
      }
    }
    poll()
    return () => {
      stopped = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, phase])

  const handleCancel = async () => {
    try {
      await api.qrCancel()
    } catch {
      /* 忽略 */
    }
    onClose()
  }

  return (
    <Modal
      title="115 扫码登录"
      open={open}
      onCancel={handleCancel}
      footer={null}
      destroyOnHidden
      width={340}
      centered
    >
      {phase === 'loading' && (
        <div style={{ textAlign: 'center', padding: '32px 0' }}>
          <Spin size="large" />
          <div style={{ marginTop: 12, color: '#888' }}>正在获取二维码…</div>
        </div>
      )}

      {phase === 'error' && (
        <Result
          status="warning"
          title="获取二维码失败"
          subTitle={errMsg}
          extra={
            <Button icon={<ReloadOutlined />} onClick={() => { createSession(); setReloadKey(k => k + 1) }}>
              重试
            </Button>
          }
        />
      )}

      {phase === 'expired' && (
        <Result
          status="info"
          title="二维码已过期"
          subTitle="请重新获取"
          extra={
            <Button type="primary" icon={<ReloadOutlined />} onClick={createSession}>
              刷新二维码
            </Button>
          }
        />
      )}

      {phase === 'confirmed' && (
        <Result
          status="success"
          title="登录成功"
          subTitle="Cookie 已自动保存到配置"
          extra={<Button type="primary" onClick={onClose}>完成</Button>}
        />
      )}

      {(phase === 'waiting' || phase === 'scanned') && (
        <div style={{ textAlign: 'center', padding: '12px 0 4px' }}>
          <div
            style={{
              display: 'inline-block',
              padding: 12,
              borderRadius: 12,
              border: '1px solid #eee',
              filter: phase === 'scanned' ? 'blur(3px) opacity(0.4)' : 'none',
              transition: 'filter .3s',
              position: 'relative',
            }}
          >
            <img src={qrDataUrl} alt="115 登录二维码" width={240} height={240} />
            {phase === 'scanned' && (
              <div
                style={{
                  position: 'absolute',
                  inset: 0,
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: 8,
                }}
              >
                <div style={{ fontSize: 40 }}>✅</div>
                <div style={{ fontWeight: 600 }}>已扫码</div>
                <div style={{ color: '#888', fontSize: 12 }}>请在手机上点击确认</div>
              </div>
            )}
          </div>
          <div style={{ marginTop: 16, color: phase === 'scanned' ? '#52c41a' : '#888' }}>
            {phase === 'waiting' ? '请用 115 手机 App 扫码' : '扫码成功，等待确认…'}
          </div>
          <Button size="small" type="link" onClick={createSession} key={reloadKey}>
            刷新二维码
          </Button>
        </div>
      )}
    </Modal>
  )
}
