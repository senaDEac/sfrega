package org.gioui;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.graphics.PixelFormat;
import android.os.Build;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.IBinder;
import android.os.Looper;
import android.provider.Settings;
import android.view.Gravity;
import android.view.MotionEvent;
import android.view.View;
import android.view.WindowManager;

import java.io.File;

/**
 * OverlayService hospeda um segundo GioView como janela TYPE_APPLICATION_OVERLAY.
 * O lado Go sinaliza pelo arquivo "ovl.flag" no filesDir: existe => overlay
 * visível; removido => este serviço remove a view e morre.
 */
public class OverlayService extends Service {
	private static final String FLAG = "ovl.flag";
	private static final int DRAG_PX = 30;

	private WindowManager wm;
	private GioView view;
	private WindowManager.LayoutParams params;
	private Handler poller;
	private boolean starting;

	public static boolean canOverlay(Context c) {
		return Build.VERSION.SDK_INT < 23 || Settings.canDrawOverlays(c);
	}

	private static boolean permAsked;

	/** Chamado pela GioActivity: inicia o serviço se a flag existir e a permissão estiver ok. */
	public static void maybeStart(Context c) {
		boolean flag = new File(c.getFilesDir(), FLAG).exists();
		if (!flag) {
			permAsked = false;
			return;
		}
		if (!canOverlay(c)) {
			// permissão "exibir sobre outros apps" ausente: abre a tela de
			// configuração uma vez por pedido de overlay
			if (!permAsked) {
				permAsked = true;
				Intent i = new Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
					android.net.Uri.parse("package:" + c.getPackageName()));
				i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
				try { c.startActivity(i); } catch (Throwable ignored) {}
			}
			return;
		}
		permAsked = false;
		Intent i = new Intent(c, OverlayService.class);
		if (Build.VERSION.SDK_INT >= 26) {
			c.startForegroundService(i);
		} else {
			c.startService(i);
		}
	}

	@Override public int onStartCommand(Intent intent, int flags, int startId) {
		if (!canOverlay(this)) {
			stopSelf();
			return START_NOT_STICKY;
		}
		if (view != null || starting) {
			return START_STICKY;
		}
		starting = true;
		startForeground(1, buildNotif());
		// new GioView() bloqueia no rendezvous do Go até a janela ser pedida —
		// fora da main thread para não dar ANR.
		HandlerThread ht = new HandlerThread("ovl-view");
		ht.start();
		new Handler(ht.getLooper()).post(new Runnable() {
			@Override public void run() { setupView(); }
		});
		startPoller();
		return START_STICKY;
	}

	private void setupView() {
		try {
			wm = (WindowManager) getSystemService(WINDOW_SERVICE);
			view = new GioView(this);
			android.util.DisplayMetrics dm = getResources().getDisplayMetrics();
			int w = (int) (dm.widthPixels * 0.92f);
			int barH = (int) Math.ceil(18 * (w / 275f));
			int h = DRAG_PX + barH + 12;
			int type = Build.VERSION.SDK_INT >= 26
				? WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
				: WindowManager.LayoutParams.TYPE_PHONE;
			params = new WindowManager.LayoutParams(w, h, type,
				WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL,
				PixelFormat.TRANSLUCENT);
			params.gravity = Gravity.TOP | Gravity.CENTER_HORIZONTAL;
			params.y = 140;
			wm.addView(view, params);
			// equivalente a onStart/onResume da Activity: sem isso o Go nunca
			// recebe FrameEvents (w.started/w.visible ficam falsos)
			view.start();
			view.resume();
			view.setOnTouchListener(new View.OnTouchListener() {
				private float downX, downY;
				private int startX, startY;
				private boolean dragging;
				@Override public boolean onTouch(View v, MotionEvent ev) {
					switch (ev.getAction()) {
					case MotionEvent.ACTION_DOWN:
						if (ev.getY() > DRAG_PX) return false;
						downX = ev.getRawX(); downY = ev.getRawY();
						startX = params.x; startY = params.y;
						dragging = true;
						return true;
					case MotionEvent.ACTION_MOVE:
						if (!dragging) return false;
						params.x = startX + (int) (ev.getRawX() - downX);
						params.y = startY + (int) (ev.getRawY() - downY);
						wm.updateViewLayout(view, params);
						return true;
					case MotionEvent.ACTION_UP:
					case MotionEvent.ACTION_CANCEL:
						dragging = false;
						return true;
					}
					return false;
				}
			});
		} catch (Throwable t) {
			t.printStackTrace();
			starting = false;
			stopSelf();
		}
	}

	private void startPoller() {
		poller = new Handler(Looper.getMainLooper());
		poller.postDelayed(new Runnable() {
			@Override public void run() {
				if (!new File(getFilesDir(), FLAG).exists()) {
					cleanup();
					return;
				}
				poller.postDelayed(this, 500);
			}
		}, 500);
	}

	private void cleanup() {
		try {
			if (view != null) {
				view.destroy();
				if (wm != null) {
					wm.removeView(view);
				}
			}
		} catch (Throwable ignored) {
		}
		view = null;
		stopForeground(true);
		stopSelf();
	}

	@Override public void onDestroy() {
		if (poller != null) poller.removeCallbacksAndMessages(null);
		cleanup();
		super.onDestroy();
	}

	@Override public IBinder onBind(Intent intent) { return null; }

	private Notification buildNotif() {
		String ch = "sfrega_overlay";
		NotificationManager nm = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
		if (Build.VERSION.SDK_INT >= 26) {
			nm.createNotificationChannel(new NotificationChannel(ch, "sfrega",
				NotificationManager.IMPORTANCE_LOW));
		}
		Intent open = new Intent(this, GioActivity.class);
		PendingIntent pi = PendingIntent.getActivity(this, 0, open,
			PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
		Notification.Builder b = Build.VERSION.SDK_INT >= 26
			? new Notification.Builder(this, ch)
			: new Notification.Builder(this);
		return b.setContentTitle("sfrega")
			.setContentText("mini player ativo")
			.setSmallIcon(android.R.drawable.ic_media_play)
			.setContentIntent(pi)
			.build();
	}
}
